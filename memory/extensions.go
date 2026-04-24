/*
Memory Management Extensions
=============================

Advanced memory management implementations:
  - Demand paging with page fault handling (MEM-001)
  - Working set model with thrashing detection (MEM-002)
  - Buddy system allocator (MEM-003)
  - Two-level page table (MEM-004)
*/

package memory

import (
	"errors"
	"fmt"
	"math/bits"
	"sync"
	"sync/atomic"
)

// =============================================================================
// MEM-001: Demand Paging Simulation
// =============================================================================

// DiskStore simulates a backing store (disk) for pages.
type DiskStore struct {
	mu      sync.RWMutex
	pages   map[int][]byte // pageNum -> page data
	latency int            // simulated disk latency (access count units)
}

// NewDiskStore creates a disk with the given simulated latency.
func NewDiskStore(latency int) *DiskStore {
	return &DiskStore{pages: make(map[int][]byte), latency: latency}
}

// StorePage writes a page to disk (called on eviction if dirty).
func (d *DiskStore) StorePage(pageNum int, data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	d.pages[pageNum] = cp
}

// LoadPage reads a page from disk; returns zeroed data for never-stored pages.
func (d *DiskStore) LoadPage(pageNum int) []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if data, ok := d.pages[pageNum]; ok {
		cp := make([]byte, len(data))
		copy(cp, data)
		return cp
	}
	return make([]byte, PageSize)
}

// HasPage reports whether a page has ever been stored.
func (d *DiskStore) HasPage(pageNum int) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.pages[pageNum]
	return ok
}

// PageSize is the byte size of each simulated page.
const PageSize = 4096

// DemandPagingFrame holds a resident page plus its data.
type DemandPagingFrame struct {
	pageNum int
	dirty   bool
	data    []byte
}

// DemandPagingSimulator simulates demand paging with a fixed physical memory.
// Page faults trigger disk I/O; eviction uses a simple LRU policy.
type DemandPagingSimulator struct {
	mu sync.Mutex

	numFrames  int
	frames     []*DemandPagingFrame // resident pages (nil = empty)
	pageToSlot map[int]int          // pageNum -> frame slot
	lruOrder   []int                // front = most recent

	disk *DiskStore

	// Metrics
	PageFaults  int
	DiskReads   int
	DiskWrites  int
	TotalAccess int
}

// NewDemandPagingSimulator creates a simulator with numFrames physical frames.
func NewDemandPagingSimulator(numFrames int, disk *DiskStore) *DemandPagingSimulator {
	return &DemandPagingSimulator{
		numFrames:  numFrames,
		frames:     make([]*DemandPagingFrame, numFrames),
		pageToSlot: make(map[int]int),
		disk:       disk,
	}
}

// Access simulates a memory access to the given page.
// Returns (pageFault bool, evictedPage int (-1 if none)).
func (s *DemandPagingSimulator) Access(pageNum int) (pageFault bool, evictedPage int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalAccess++
	evictedPage = -1

	if slot, ok := s.pageToSlot[pageNum]; ok {
		// Page hit — update LRU order.
		s.touchLRU(pageNum)
		_ = slot
		return false, -1
	}

	// Page fault.
	s.PageFaults++
	pageFault = true

	// Find a free frame or evict.
	slot := s.findFreeSlot()
	if slot == -1 {
		// Evict LRU page.
		victim := s.lruOrder[len(s.lruOrder)-1]
		slot = s.pageToSlot[victim]
		evictedPage = victim
		if s.frames[slot].dirty {
			s.disk.StorePage(victim, s.frames[slot].data)
			s.DiskWrites++
		}
		delete(s.pageToSlot, victim)
		s.lruOrder = s.lruOrder[:len(s.lruOrder)-1]
	}

	// Load from disk.
	data := s.disk.LoadPage(pageNum)
	s.DiskReads++
	s.frames[slot] = &DemandPagingFrame{pageNum: pageNum, data: data}
	s.pageToSlot[pageNum] = slot
	s.lruOrder = append([]int{pageNum}, s.lruOrder...)

	return true, evictedPage
}

// Write simulates a write to a page, marking it dirty.
func (s *DemandPagingSimulator) Write(pageNum int, data []byte) (pageFault bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalAccess++

	if slot, ok := s.pageToSlot[pageNum]; ok {
		s.frames[slot].dirty = true
		if len(data) <= len(s.frames[slot].data) {
			copy(s.frames[slot].data, data)
		}
		s.touchLRU(pageNum)
		return false
	}

	// Page fault — load then mark dirty.
	s.PageFaults++
	slot := s.findFreeSlot()
	if slot == -1 {
		victim := s.lruOrder[len(s.lruOrder)-1]
		slot = s.pageToSlot[victim]
		if s.frames[slot].dirty {
			s.disk.StorePage(victim, s.frames[slot].data)
			s.DiskWrites++
		}
		delete(s.pageToSlot, victim)
		s.lruOrder = s.lruOrder[:len(s.lruOrder)-1]
	}
	pageData := s.disk.LoadPage(pageNum)
	s.DiskReads++
	if len(data) <= len(pageData) {
		copy(pageData, data)
	}
	s.frames[slot] = &DemandPagingFrame{pageNum: pageNum, dirty: true, data: pageData}
	s.pageToSlot[pageNum] = slot
	s.lruOrder = append([]int{pageNum}, s.lruOrder...)
	return true
}

// ReadPage returns a copy of the in-memory data for a resident page.
// Returns an error if the page is not currently resident.
func (s *DemandPagingSimulator) ReadPage(pageNum int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.pageToSlot[pageNum]
	if !ok {
		return nil, fmt.Errorf("page %d not resident", pageNum)
	}
	cp := make([]byte, len(s.frames[slot].data))
	copy(cp, s.frames[slot].data)
	return cp, nil
}

// FlushAll writes all dirty pages to disk.
func (s *DemandPagingSimulator) FlushAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, slot := range s.pageToSlot {
		f := s.frames[slot]
		if f != nil && f.dirty {
			s.disk.StorePage(f.pageNum, f.data)
			s.DiskWrites++
			f.dirty = false
		}
	}
}

// ResidentSet returns the set of currently resident page numbers.
func (s *DemandPagingSimulator) ResidentSet() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	pages := make([]int, 0, len(s.pageToSlot))
	for p := range s.pageToSlot {
		pages = append(pages, p)
	}
	return pages
}

// PageFaultRate returns page faults / total accesses.
func (s *DemandPagingSimulator) PageFaultRate() float64 {
	if s.TotalAccess == 0 {
		return 0
	}
	return float64(s.PageFaults) / float64(s.TotalAccess)
}

func (s *DemandPagingSimulator) findFreeSlot() int {
	for i, f := range s.frames {
		if f == nil {
			return i
		}
	}
	return -1
}

func (s *DemandPagingSimulator) touchLRU(pageNum int) {
	for i, p := range s.lruOrder {
		if p == pageNum {
			s.lruOrder = append(s.lruOrder[:i], s.lruOrder[i+1:]...)
			break
		}
	}
	s.lruOrder = append([]int{pageNum}, s.lruOrder...)
}

// =============================================================================
// MEM-002: Working Set Model
// =============================================================================

// WorkingSetModel tracks the working set of a process over a sliding window
// of the last Δ memory accesses.
type WorkingSetModel struct {
	mu      sync.Mutex
	delta   int      // window size (number of references)
	history []int    // ring of last Δ page references
	head    int      // next write position
	count   int      // how many entries filled so far
	frames  int      // physical frames allocated to this process
}

// NewWorkingSetModel creates a working set tracker with the given window (Δ)
// and the number of physical frames allocated to the process.
func NewWorkingSetModel(delta, frames int) *WorkingSetModel {
	return &WorkingSetModel{
		delta:   delta,
		history: make([]int, delta),
		frames:  frames,
	}
}

// Access records a reference to pageNum.
func (w *WorkingSetModel) Access(pageNum int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.history[w.head] = pageNum
	w.head = (w.head + 1) % w.delta
	if w.count < w.delta {
		w.count++
	}
}

// WorkingSetSize returns the number of distinct pages referenced in the last Δ accesses.
func (w *WorkingSetModel) WorkingSetSize() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.distinctPages()
}

// WorkingSet returns the current working set (distinct pages in the last Δ references).
func (w *WorkingSetModel) WorkingSet() []int {
	w.mu.Lock()
	defer w.mu.Unlock()
	seen := make(map[int]struct{})
	n := w.count
	// Walk backwards from (head-1) for the last n entries.
	for i := 0; i < n; i++ {
		idx := (w.head - 1 - i + w.delta) % w.delta
		seen[w.history[idx]] = struct{}{}
	}
	result := make([]int, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	return result
}

// IsThrashing returns true when the working set size exceeds available frames,
// indicating more pages are needed than can be held in memory.
func (w *WorkingSetModel) IsThrashing() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.distinctPages() > w.frames
}

// ThrashingPressure returns the ratio workingSetSize/frames (>1 means thrashing).
func (w *WorkingSetModel) ThrashingPressure() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.frames == 0 {
		return 0
	}
	return float64(w.distinctPages()) / float64(w.frames)
}

func (w *WorkingSetModel) distinctPages() int {
	seen := make(map[int]struct{})
	for i := 0; i < w.count; i++ {
		idx := (w.head - 1 - i + w.delta) % w.delta
		seen[w.history[idx]] = struct{}{}
	}
	return len(seen)
}

// =============================================================================
// MEM-003: Buddy System Allocator
// =============================================================================

// buddyBlock represents one block in the buddy system.
type buddyBlock struct {
	addr int
	size int
	free bool
	next *buddyBlock
}

// BuddyAllocator implements the buddy system memory allocator.
// Memory is divided into power-of-2 sized blocks. On allocation the smallest
// sufficient block is split until it fits; on free the buddy is coalesced.
type BuddyAllocator struct {
	mu       sync.Mutex
	totalSize int
	minOrder  int // minimum block order (2^minOrder bytes)
	maxOrder  int // maximum block order (= log2(totalSize))
	// freeLists[k] holds free blocks of size 2^(minOrder+k)
	freeLists []map[int]*buddyBlock // addr -> block
	// allBlocks tracks all allocated blocks by address
	allocated map[int]int // addr -> order

	// Metrics
	TotalAllocations int
	TotalFrees       int
	InternalFrag     int // bytes wasted inside allocations
}

// NewBuddyAllocator creates an allocator managing totalSize bytes.
// totalSize must be a power of 2. minBlockSize is the minimum allocation unit.
func NewBuddyAllocator(totalSize, minBlockSize int) (*BuddyAllocator, error) {
	if totalSize <= 0 || !isPow2(totalSize) {
		return nil, errors.New("totalSize must be a positive power of 2")
	}
	if minBlockSize <= 0 || !isPow2(minBlockSize) {
		return nil, errors.New("minBlockSize must be a positive power of 2")
	}
	if minBlockSize > totalSize {
		return nil, errors.New("minBlockSize must be ≤ totalSize")
	}
	minOrder := log2(minBlockSize)
	maxOrder := log2(totalSize)
	levels := maxOrder - minOrder + 1
	freeLists := make([]map[int]*buddyBlock, levels)
	for i := range freeLists {
		freeLists[i] = make(map[int]*buddyBlock)
	}
	// Start with one free block of the full size.
	root := &buddyBlock{addr: 0, size: totalSize, free: true}
	freeLists[levels-1][0] = root

	return &BuddyAllocator{
		totalSize: totalSize,
		minOrder:  minOrder,
		maxOrder:  maxOrder,
		freeLists: freeLists,
		allocated: make(map[int]int),
	}, nil
}

// Allocate returns the start address of a free block of at least `size` bytes.
// The block is the smallest power-of-2 ≥ size.
func (b *BuddyAllocator) Allocate(size int) (int, error) {
	if size <= 0 {
		return 0, errors.New("size must be positive")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// Round up to the next power of 2 and find the order.
	blockSize := nextPow2(size)
	if blockSize < (1 << b.minOrder) {
		blockSize = 1 << b.minOrder
	}
	order := log2(blockSize)
	if order > b.maxOrder {
		return 0, fmt.Errorf("request size %d exceeds allocator capacity %d", size, b.totalSize)
	}

	// Find the smallest free list that can satisfy the request.
	level := order - b.minOrder
	foundLevel := -1
	for l := level; l < len(b.freeLists); l++ {
		if len(b.freeLists[l]) > 0 {
			foundLevel = l
			break
		}
	}
	if foundLevel == -1 {
		return 0, errors.New("out of memory")
	}

	// Take any block from foundLevel.
	var block *buddyBlock
	for _, blk := range b.freeLists[foundLevel] {
		block = blk
		break
	}
	delete(b.freeLists[foundLevel], block.addr)
	block.free = false

	// Split down to the requested level.
	for foundLevel > level {
		foundLevel--
		halfSize := block.size / 2
		buddy := &buddyBlock{addr: block.addr + halfSize, size: halfSize, free: true}
		b.freeLists[foundLevel][buddy.addr] = buddy
		block.size = halfSize
	}

	b.allocated[block.addr] = order
	b.TotalAllocations++
	b.InternalFrag += blockSize - size
	return block.addr, nil
}

// Free releases the block at addr (which must have been returned by Allocate).
// size must match the original request size to determine the block order.
func (b *BuddyAllocator) Free(addr, size int) error {
	if size <= 0 {
		return errors.New("size must be positive")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	order, ok := b.allocated[addr]
	if !ok {
		return fmt.Errorf("address %d was not allocated", addr)
	}
	blockSize := 1 << order
	delete(b.allocated, addr)
	b.TotalFrees++
	// Undo internal frag accounting.
	b.InternalFrag -= blockSize - nextPow2(size)
	if b.InternalFrag < 0 {
		b.InternalFrag = 0
	}

	// Coalesce with buddy repeatedly.
	level := order - b.minOrder
	currentAddr := addr
	currentSize := blockSize

	for level < len(b.freeLists)-1 {
		buddyAddr := b.buddyAddr(currentAddr, currentSize)
		buddy, exists := b.freeLists[level][buddyAddr]
		if !exists {
			break
		}
		// Merge with buddy.
		delete(b.freeLists[level], buddyAddr)
		if buddyAddr < currentAddr {
			currentAddr = buddyAddr
		}
		currentSize *= 2
		level++
		_ = buddy
	}

	b.freeLists[level][currentAddr] = &buddyBlock{
		addr: currentAddr,
		size: currentSize,
		free: true,
	}
	return nil
}

// FreeMemory returns the total bytes available for allocation.
func (b *BuddyAllocator) FreeMemory() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := 0
	for l, fl := range b.freeLists {
		sz := 1 << (b.minOrder + l)
		total += len(fl) * sz
	}
	return total
}

// UsedMemory returns the total bytes currently allocated.
func (b *BuddyAllocator) UsedMemory() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	used := 0
	for addr, order := range b.allocated {
		_ = addr
		used += 1 << order
	}
	return used
}

// ExternalFragmentation returns the fraction of free memory that is fragmented
// (i.e., not in the single largest free block).
func (b *BuddyAllocator) ExternalFragmentation() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	totalFree := 0
	maxFree := 0
	for l, fl := range b.freeLists {
		sz := 1 << (b.minOrder + l)
		totalFree += len(fl) * sz
		if len(fl) > 0 && sz > maxFree {
			maxFree = sz
		}
	}
	if totalFree == 0 {
		return 0
	}
	return float64(totalFree-maxFree) / float64(totalFree)
}

func (b *BuddyAllocator) buddyAddr(addr, size int) int {
	return addr ^ size // XOR flips the bit at position log2(size)
}

func isPow2(n int) bool {
	return n > 0 && n&(n-1) == 0
}

func log2(n int) int {
	return bits.Len(uint(n)) - 1
}

func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(n-1))
}

// =============================================================================
// MEM-004: Two-Level Page Table
// =============================================================================

// TwoLevelPageTable implements a hierarchical two-level page table.
//
// Virtual address layout (configurable bit widths):
//
//	[ PD index | PT index | offset ]
//
// PD entries are created eagerly; PT entries are created on first mapping.
type TwoLevelPageTable struct {
	mu sync.RWMutex

	pdBits     int // bits for PD index
	ptBits     int // bits for PT index
	offsetBits int // bits for page offset

	pdMask     int
	ptMask     int
	offsetMask int

	pd [][]int // pd[pdIndex][ptIndex] = frame number (-1 = not mapped)

	nextFrame  int
	frameCount int

	// Metrics (accessed atomically)
	translations   int64
	pageFaultCount int64
	mappingCount   int64
}

// Translations returns the total number of address translations attempted.
func (t *TwoLevelPageTable) Translations() int64 { return atomic.LoadInt64(&t.translations) }

// PageFaultCount returns the number of page faults encountered during translation.
func (t *TwoLevelPageTable) PageFaultCount() int64 { return atomic.LoadInt64(&t.pageFaultCount) }

// MappingCount returns the number of page mappings installed.
func (t *TwoLevelPageTable) MappingCount() int64 { return atomic.LoadInt64(&t.mappingCount) }

// NewTwoLevelPageTable creates a two-level page table.
// pdBits + ptBits + offsetBits must equal the virtual address width.
// frameCount is the number of available physical frames.
func NewTwoLevelPageTable(pdBits, ptBits, offsetBits, frameCount int) *TwoLevelPageTable {
	pdSize := 1 << pdBits
	return &TwoLevelPageTable{
		pdBits:     pdBits,
		ptBits:     ptBits,
		offsetBits: offsetBits,
		pdMask:     (1<<pdBits - 1) << (ptBits + offsetBits),
		ptMask:     (1<<ptBits - 1) << offsetBits,
		offsetMask: 1<<offsetBits - 1,
		pd:         make([][]int, pdSize),
		frameCount: frameCount,
	}
}

// Map installs a mapping from a virtual page to the given physical frame.
// virtualAddr is any address within the target page.
func (t *TwoLevelPageTable) Map(virtualAddr, frameNum int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	pdIdx, ptIdx := t.splitVA(virtualAddr)

	if t.pd[pdIdx] == nil {
		ptSize := 1 << t.ptBits
		t.pd[pdIdx] = make([]int, ptSize)
		for i := range t.pd[pdIdx] {
			t.pd[pdIdx][i] = -1
		}
	}
	t.pd[pdIdx][ptIdx] = frameNum
	atomic.AddInt64(&t.mappingCount, 1)
	return nil
}

// Unmap removes the mapping for the virtual page containing virtualAddr.
func (t *TwoLevelPageTable) Unmap(virtualAddr int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pdIdx, ptIdx := t.splitVA(virtualAddr)
	if t.pd[pdIdx] == nil {
		return
	}
	t.pd[pdIdx][ptIdx] = -1
}

// Translate converts a virtual address to a physical address.
// Returns ErrPageFault if the virtual page is not mapped.
func (t *TwoLevelPageTable) Translate(virtualAddr int) (physicalAddr int, err error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	atomic.AddInt64(&t.translations, 1)
	pdIdx, ptIdx := t.splitVA(virtualAddr)
	offset := virtualAddr & t.offsetMask

	if t.pd[pdIdx] == nil {
		atomic.AddInt64(&t.pageFaultCount, 1)
		return 0, fmt.Errorf("page fault: PD entry %d not present (vaddr=0x%x)", pdIdx, virtualAddr)
	}
	frameNum := t.pd[pdIdx][ptIdx]
	if frameNum == -1 {
		atomic.AddInt64(&t.pageFaultCount, 1)
		return 0, fmt.Errorf("page fault: PT entry [%d][%d] not present (vaddr=0x%x)", pdIdx, ptIdx, virtualAddr)
	}
	pageSize := 1 << t.offsetBits
	return frameNum*pageSize + offset, nil
}

// AllocateAndMap allocates the next free frame and maps it to the virtual page
// containing virtualAddr. Returns the assigned physical frame number.
func (t *TwoLevelPageTable) AllocateAndMap(virtualAddr int) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.nextFrame >= t.frameCount {
		return 0, errors.New("out of physical frames")
	}
	frame := t.nextFrame
	t.nextFrame++

	pdIdx, ptIdx := t.splitVA(virtualAddr)
	if t.pd[pdIdx] == nil {
		ptSize := 1 << t.ptBits
		t.pd[pdIdx] = make([]int, ptSize)
		for i := range t.pd[pdIdx] {
			t.pd[pdIdx][i] = -1
		}
	}
	t.pd[pdIdx][ptIdx] = frame
	atomic.AddInt64(&t.mappingCount, 1)
	return frame, nil
}

// MappedPages returns the number of currently mapped virtual pages.
func (t *TwoLevelPageTable) MappedPages() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := 0
	for _, pt := range t.pd {
		if pt == nil {
			continue
		}
		for _, frame := range pt {
			if frame != -1 {
				count++
			}
		}
	}
	return count
}

// PDEntries returns the number of allocated page-directory entries (non-nil PTs).
func (t *TwoLevelPageTable) PDEntries() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := 0
	for _, pt := range t.pd {
		if pt != nil {
			n++
		}
	}
	return n
}

func (t *TwoLevelPageTable) splitVA(virtualAddr int) (pdIdx, ptIdx int) {
	pdIdx = (virtualAddr & t.pdMask) >> (t.ptBits + t.offsetBits)
	ptIdx = (virtualAddr & t.ptMask) >> t.offsetBits
	return
}
