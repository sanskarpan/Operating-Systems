/*
Memory Management
==================

Implementation of memory management strategies including paging, segmentation,
and page replacement algorithms.

Applications:
- Virtual memory management
- Cache replacement policies
- Memory allocation in operating systems
- Buffer pool management in databases
*/

package memory

import (
	"container/list"
	"errors"
	"fmt"
	"math"
)

// =============================================================================
// Page and Frame Management
// =============================================================================

// Page represents a page in virtual memory
type Page struct {
	Number      int
	Referenced  bool
	Modified    bool
	LoadTime    int
	LastAccess  int
	AccessCount int
}

// Frame represents a physical memory frame
type Frame struct {
	Number      int
	Page        *Page
	Occupied    bool
	LastAccess  int
}

// PageTable maps virtual pages to physical frames
type PageTable struct {
	Entries map[int]*Frame // Page number -> Frame
}

// NewPageTable creates a new page table
func NewPageTable() *PageTable {
	return &PageTable{
		Entries: make(map[int]*Frame),
	}
}

// Lookup finds the frame for a given page
func (pt *PageTable) Lookup(pageNum int) (*Frame, bool) {
	frame, exists := pt.Entries[pageNum]
	return frame, exists
}

// Insert adds a page-to-frame mapping
func (pt *PageTable) Insert(pageNum int, frame *Frame) {
	pt.Entries[pageNum] = frame
}

// Remove removes a page-to-frame mapping
func (pt *PageTable) Remove(pageNum int) {
	delete(pt.Entries, pageNum)
}

// =============================================================================
// Page Replacement Algorithms
// =============================================================================

// PageReplacementAlgorithm interface for different algorithms
type PageReplacementAlgorithm interface {
	Access(pageNum int, currentTime int) (bool, int) // Returns (hit, evictedPage)
	Name() string
	Reset()
	GetPageFaults() int
}

// =============================================================================
// FIFO (First-In-First-Out) Page Replacement
// =============================================================================

// FIFO implements First-In-First-Out page replacement
type FIFO struct {
	Capacity   int
	Frames     []int
	PageSet    map[int]bool
	Queue      *list.List
	PageFaults int
}

// NewFIFO creates a new FIFO page replacement instance
func NewFIFO(capacity int) *FIFO {
	return &FIFO{
		Capacity: capacity,
		Frames:   make([]int, 0, capacity),
		PageSet:  make(map[int]bool),
		Queue:    list.New(),
	}
}

func (f *FIFO) Name() string {
	return "FIFO (First-In-First-Out)"
}

func (f *FIFO) Access(pageNum int, currentTime int) (bool, int) {
	// Check if page already in memory
	if f.PageSet[pageNum] {
		return true, -1 // Page hit
	}

	// Page fault
	f.PageFaults++
	evictedPage := -1

	if len(f.Frames) < f.Capacity {
		// Still have free frames
		f.Frames = append(f.Frames, pageNum)
		f.PageSet[pageNum] = true
		f.Queue.PushBack(pageNum)
	} else {
		// Need to evict oldest page
		oldest := f.Queue.Front()
		evictedPage = oldest.Value.(int)
		f.Queue.Remove(oldest)
		delete(f.PageSet, evictedPage)

		// Add new page
		f.PageSet[pageNum] = true
		f.Queue.PushBack(pageNum)
	}

	return false, evictedPage
}

func (f *FIFO) Reset() {
	f.Frames = make([]int, 0, f.Capacity)
	f.PageSet = make(map[int]bool)
	f.Queue = list.New()
	f.PageFaults = 0
}

func (f *FIFO) GetPageFaults() int {
	return f.PageFaults
}

// =============================================================================
// LRU (Least Recently Used) Page Replacement
// =============================================================================

// LRU implements Least Recently Used page replacement
type LRU struct {
	Capacity   int
	PageMap    map[int]*list.Element
	List       *list.List
	PageFaults int
}

type lruEntry struct {
	pageNum    int
	accessTime int
}

// NewLRU creates a new LRU page replacement instance
func NewLRU(capacity int) *LRU {
	return &LRU{
		Capacity: capacity,
		PageMap:  make(map[int]*list.Element),
		List:     list.New(),
	}
}

func (l *LRU) Name() string {
	return "LRU (Least Recently Used)"
}

func (l *LRU) Access(pageNum int, currentTime int) (bool, int) {
	// Check if page in memory
	if elem, exists := l.PageMap[pageNum]; exists {
		// Page hit - move to front (most recently used)
		l.List.MoveToFront(elem)
		elem.Value.(*lruEntry).accessTime = currentTime
		return true, -1
	}

	// Page fault
	l.PageFaults++
	evictedPage := -1

	if l.List.Len() < l.Capacity {
		// Still have free frames
		entry := &lruEntry{pageNum: pageNum, accessTime: currentTime}
		elem := l.List.PushFront(entry)
		l.PageMap[pageNum] = elem
	} else {
		// Evict least recently used (back of list)
		back := l.List.Back()
		evictedPage = back.Value.(*lruEntry).pageNum
		l.List.Remove(back)
		delete(l.PageMap, evictedPage)

		// Add new page
		entry := &lruEntry{pageNum: pageNum, accessTime: currentTime}
		elem := l.List.PushFront(entry)
		l.PageMap[pageNum] = elem
	}

	return false, evictedPage
}

func (l *LRU) Reset() {
	l.PageMap = make(map[int]*list.Element)
	l.List = list.New()
	l.PageFaults = 0
}

func (l *LRU) GetPageFaults() int {
	return l.PageFaults
}

// =============================================================================
// Optimal Page Replacement (for comparison)
// =============================================================================

// Optimal implements optimal page replacement (requires future knowledge)
type Optimal struct {
	Capacity      int
	Frames        map[int]bool
	ReferenceStr  []int // Future reference string
	CurrentIndex  int
	PageFaults    int
}

// NewOptimal creates an optimal page replacement instance
func NewOptimal(capacity int, referenceString []int) *Optimal {
	return &Optimal{
		Capacity:     capacity,
		Frames:       make(map[int]bool),
		ReferenceStr: referenceString,
		CurrentIndex: 0,
	}
}

func (o *Optimal) Name() string {
	return "Optimal (Theoretical)"
}

func (o *Optimal) Access(pageNum int, currentTime int) (bool, int) {
	// Check if page in memory
	if o.Frames[pageNum] {
		o.CurrentIndex++
		return true, -1
	}

	// Page fault
	o.PageFaults++
	evictedPage := -1

	if len(o.Frames) < o.Capacity {
		// Still have free frames
		o.Frames[pageNum] = true
	} else {
		// Find page that won't be used for longest time
		farthest := o.CurrentIndex
		pageToEvict := -1

		for page := range o.Frames {
			nextUse := o.findNextUse(page, o.CurrentIndex+1)
			if nextUse > farthest {
				farthest = nextUse
				pageToEvict = page
			}
		}

		if pageToEvict != -1 {
			evictedPage = pageToEvict
			delete(o.Frames, pageToEvict)
		}

		o.Frames[pageNum] = true
	}

	o.CurrentIndex++
	return false, evictedPage
}

func (o *Optimal) findNextUse(pageNum, startIndex int) int {
	for i := startIndex; i < len(o.ReferenceStr); i++ {
		if o.ReferenceStr[i] == pageNum {
			return i
		}
	}
	return len(o.ReferenceStr) + 1 // Never used again
}

func (o *Optimal) Reset() {
	o.Frames = make(map[int]bool)
	o.CurrentIndex = 0
	o.PageFaults = 0
}

func (o *Optimal) GetPageFaults() int {
	return o.PageFaults
}

// =============================================================================
// Clock (Second Chance) Page Replacement
// =============================================================================

// Clock implements clock/second chance page replacement
type Clock struct {
	Capacity      int
	Frames        []int
	ReferenceBits []bool
	Pointer       int
	PageSet       map[int]int // Page -> index in frames
	PageFaults    int
}

// NewClock creates a clock page replacement instance
func NewClock(capacity int) *Clock {
	return &Clock{
		Capacity:      capacity,
		Frames:        make([]int, 0, capacity),
		ReferenceBits: make([]bool, 0, capacity),
		Pointer:       0,
		PageSet:       make(map[int]int),
	}
}

func (c *Clock) Name() string {
	return "Clock (Second Chance)"
}

func (c *Clock) Access(pageNum int, currentTime int) (bool, int) {
	// Check if page in memory
	if idx, exists := c.PageSet[pageNum]; exists {
		c.ReferenceBits[idx] = true
		return true, -1
	}

	// Page fault
	c.PageFaults++
	evictedPage := -1

	if len(c.Frames) < c.Capacity {
		// Still have free frames
		idx := len(c.Frames)
		c.Frames = append(c.Frames, pageNum)
		c.ReferenceBits = append(c.ReferenceBits, true)
		c.PageSet[pageNum] = idx
	} else {
		// Find victim using clock algorithm
		for {
			if !c.ReferenceBits[c.Pointer] {
				// Found victim (reference bit = 0)
				evictedPage = c.Frames[c.Pointer]
				delete(c.PageSet, evictedPage)

				c.Frames[c.Pointer] = pageNum
				c.ReferenceBits[c.Pointer] = true
				c.PageSet[pageNum] = c.Pointer

				c.Pointer = (c.Pointer + 1) % c.Capacity
				break
			} else {
				// Give second chance - clear reference bit
				c.ReferenceBits[c.Pointer] = false
				c.Pointer = (c.Pointer + 1) % c.Capacity
			}
		}
	}

	return false, evictedPage
}

func (c *Clock) Reset() {
	c.Frames = make([]int, 0, c.Capacity)
	c.ReferenceBits = make([]bool, 0, c.Capacity)
	c.Pointer = 0
	c.PageSet = make(map[int]int)
	c.PageFaults = 0
}

func (c *Clock) GetPageFaults() int {
	return c.PageFaults
}

// =============================================================================
// LRU-K Page Replacement
// =============================================================================

// LRUK implements LRU-K page replacement (evict page with oldest K-th reference)
type LRUK struct {
	Capacity   int
	K          int
	Frames     map[int]bool
	History    map[int][]int // Page -> list of K most recent access times
	PageFaults int
}

// NewLRUK creates an LRU-K page replacement instance
func NewLRUK(capacity, k int) *LRUK {
	return &LRUK{
		Capacity: capacity,
		K:        k,
		Frames:   make(map[int]bool),
		History:  make(map[int][]int),
	}
}

func (l *LRUK) Name() string {
	return fmt.Sprintf("LRU-%d", l.K)
}

func (l *LRUK) Access(pageNum int, currentTime int) (bool, int) {
	// Update access history
	if _, exists := l.History[pageNum]; !exists {
		l.History[pageNum] = make([]int, 0, l.K)
	}

	l.History[pageNum] = append(l.History[pageNum], currentTime)
	if len(l.History[pageNum]) > l.K {
		l.History[pageNum] = l.History[pageNum][1:]
	}

	// Check if page in memory
	if l.Frames[pageNum] {
		return true, -1
	}

	// Page fault
	l.PageFaults++
	evictedPage := -1

	if len(l.Frames) < l.Capacity {
		l.Frames[pageNum] = true
	} else {
		// Find page with oldest K-th reference
		oldestKthRef := currentTime + 1
		pageToEvict := -1

		for page := range l.Frames {
			history := l.History[page]
			if len(history) < l.K {
				// Not enough history - use oldest reference
				kthRef := history[0]
				if kthRef < oldestKthRef {
					oldestKthRef = kthRef
					pageToEvict = page
				}
			} else {
				// Use K-th most recent reference
				kthRef := history[0]
				if kthRef < oldestKthRef {
					oldestKthRef = kthRef
					pageToEvict = page
				}
			}
		}

		if pageToEvict != -1 {
			evictedPage = pageToEvict
			delete(l.Frames, pageToEvict)
		}

		l.Frames[pageNum] = true
	}

	return false, evictedPage
}

func (l *LRUK) Reset() {
	l.Frames = make(map[int]bool)
	l.History = make(map[int][]int)
	l.PageFaults = 0
}

func (l *LRUK) GetPageFaults() int {
	return l.PageFaults
}

// =============================================================================
// Memory Allocation Algorithms
// =============================================================================

// MemoryBlock represents a block of memory
type MemoryBlock struct {
	Start      int
	Size       int
	Allocated  bool
	ProcessID  int
}

// MemoryAllocator interface for memory allocation strategies
type MemoryAllocator interface {
	Allocate(processID, size int) (int, error) // Returns start address
	Deallocate(processID int) error
	Name() string
}

// =============================================================================
// First-Fit Allocation
// =============================================================================

// FirstFit implements first-fit memory allocation
type FirstFit struct {
	TotalSize int
	Blocks    []*MemoryBlock
}

// NewFirstFit creates a first-fit allocator
func NewFirstFit(totalSize int) *FirstFit {
	return &FirstFit{
		TotalSize: totalSize,
		Blocks: []*MemoryBlock{
			{Start: 0, Size: totalSize, Allocated: false},
		},
	}
}

func (ff *FirstFit) Name() string {
	return "First-Fit"
}

func (ff *FirstFit) Allocate(processID, size int) (int, error) {
	// Find first block large enough
	for i, block := range ff.Blocks {
		if !block.Allocated && block.Size >= size {
			startAddr := block.Start

			if block.Size == size {
				// Exact fit
				block.Allocated = true
				block.ProcessID = processID
			} else {
				// Split block
				allocated := &MemoryBlock{
					Start:     block.Start,
					Size:      size,
					Allocated: true,
					ProcessID: processID,
				}
				remaining := &MemoryBlock{
					Start:     block.Start + size,
					Size:      block.Size - size,
					Allocated: false,
				}

				// Replace block with two blocks
				ff.Blocks = append(ff.Blocks[:i], append([]*MemoryBlock{allocated, remaining}, ff.Blocks[i+1:]...)...)
			}

			return startAddr, nil
		}
	}

	return -1, errors.New("no suitable block found")
}

func (ff *FirstFit) Deallocate(processID int) error {
	for i, block := range ff.Blocks {
		if block.Allocated && block.ProcessID == processID {
			block.Allocated = false
			block.ProcessID = 0

			// Merge with adjacent free blocks
			ff.mergeBlocks(i)
			return nil
		}
	}

	return errors.New("process not found")
}

func (ff *FirstFit) mergeBlocks(index int) {
	// Merge with next block if free
	if index < len(ff.Blocks)-1 && !ff.Blocks[index+1].Allocated {
		ff.Blocks[index].Size += ff.Blocks[index+1].Size
		ff.Blocks = append(ff.Blocks[:index+1], ff.Blocks[index+2:]...)
	}

	// Merge with previous block if free
	if index > 0 && !ff.Blocks[index-1].Allocated {
		ff.Blocks[index-1].Size += ff.Blocks[index].Size
		ff.Blocks = append(ff.Blocks[:index], ff.Blocks[index+1:]...)
	}
}

// =============================================================================
// Best-Fit Allocation
// =============================================================================

// BestFit implements best-fit memory allocation
type BestFit struct {
	TotalSize int
	Blocks    []*MemoryBlock
}

// NewBestFit creates a best-fit allocator
func NewBestFit(totalSize int) *BestFit {
	return &BestFit{
		TotalSize: totalSize,
		Blocks: []*MemoryBlock{
			{Start: 0, Size: totalSize, Allocated: false},
		},
	}
}

func (bf *BestFit) Name() string {
	return "Best-Fit"
}

func (bf *BestFit) Allocate(processID, size int) (int, error) {
	// Find smallest block large enough
	bestIdx := -1
	bestSize := math.MaxInt32

	for i, block := range bf.Blocks {
		if !block.Allocated && block.Size >= size && block.Size < bestSize {
			bestIdx = i
			bestSize = block.Size
		}
	}

	if bestIdx == -1 {
		return -1, errors.New("no suitable block found")
	}

	block := bf.Blocks[bestIdx]
	startAddr := block.Start

	if block.Size == size {
		block.Allocated = true
		block.ProcessID = processID
	} else {
		// Split block
		allocated := &MemoryBlock{
			Start:     block.Start,
			Size:      size,
			Allocated: true,
			ProcessID: processID,
		}
		remaining := &MemoryBlock{
			Start:     block.Start + size,
			Size:      block.Size - size,
			Allocated: false,
		}

		bf.Blocks = append(bf.Blocks[:bestIdx], append([]*MemoryBlock{allocated, remaining}, bf.Blocks[bestIdx+1:]...)...)
	}

	return startAddr, nil
}

func (bf *BestFit) Deallocate(processID int) error {
	for i, block := range bf.Blocks {
		if block.Allocated && block.ProcessID == processID {
			block.Allocated = false
			block.ProcessID = 0
			bf.mergeBlocks(i)
			return nil
		}
	}

	return errors.New("process not found")
}

func (bf *BestFit) mergeBlocks(index int) {
	if index < len(bf.Blocks)-1 && !bf.Blocks[index+1].Allocated {
		bf.Blocks[index].Size += bf.Blocks[index+1].Size
		bf.Blocks = append(bf.Blocks[:index+1], bf.Blocks[index+2:]...)
	}

	if index > 0 && !bf.Blocks[index-1].Allocated {
		bf.Blocks[index-1].Size += bf.Blocks[index].Size
		bf.Blocks = append(bf.Blocks[:index], bf.Blocks[index+1:]...)
	}
}

// =============================================================================
// Worst-Fit Allocation
// =============================================================================

// WorstFit implements worst-fit memory allocation
type WorstFit struct {
	TotalSize int
	Blocks    []*MemoryBlock
}

// NewWorstFit creates a worst-fit allocator
func NewWorstFit(totalSize int) *WorstFit {
	return &WorstFit{
		TotalSize: totalSize,
		Blocks: []*MemoryBlock{
			{Start: 0, Size: totalSize, Allocated: false},
		},
	}
}

func (wf *WorstFit) Name() string {
	return "Worst-Fit"
}

func (wf *WorstFit) Allocate(processID, size int) (int, error) {
	// Find largest block
	worstIdx := -1
	worstSize := -1

	for i, block := range wf.Blocks {
		if !block.Allocated && block.Size >= size && block.Size > worstSize {
			worstIdx = i
			worstSize = block.Size
		}
	}

	if worstIdx == -1 {
		return -1, errors.New("no suitable block found")
	}

	block := wf.Blocks[worstIdx]
	startAddr := block.Start

	if block.Size == size {
		block.Allocated = true
		block.ProcessID = processID
	} else {
		allocated := &MemoryBlock{
			Start:     block.Start,
			Size:      size,
			Allocated: true,
			ProcessID: processID,
		}
		remaining := &MemoryBlock{
			Start:     block.Start + size,
			Size:      block.Size - size,
			Allocated: false,
		}

		wf.Blocks = append(wf.Blocks[:worstIdx], append([]*MemoryBlock{allocated, remaining}, wf.Blocks[worstIdx+1:]...)...)
	}

	return startAddr, nil
}

func (wf *WorstFit) Deallocate(processID int) error {
	for i, block := range wf.Blocks {
		if block.Allocated && block.ProcessID == processID {
			block.Allocated = false
			block.ProcessID = 0
			wf.mergeBlocks(i)
			return nil
		}
	}

	return errors.New("process not found")
}

func (wf *WorstFit) mergeBlocks(index int) {
	if index < len(wf.Blocks)-1 && !wf.Blocks[index+1].Allocated {
		wf.Blocks[index].Size += wf.Blocks[index+1].Size
		wf.Blocks = append(wf.Blocks[:index+1], wf.Blocks[index+2:]...)
	}

	if index > 0 && !wf.Blocks[index-1].Allocated {
		wf.Blocks[index-1].Size += wf.Blocks[index].Size
		wf.Blocks = append(wf.Blocks[:index], wf.Blocks[index+1:]...)
	}
}

// =============================================================================
// TLB (Translation Lookaside Buffer) Simulation
// =============================================================================

// TLBEntry represents an entry in the TLB
type TLBEntry struct {
	PageNum   int
	FrameNum  int
	Valid     bool
	LastAccess int
}

// TLB implements a translation lookaside buffer
type TLB struct {
	Entries    []*TLBEntry
	Capacity   int
	Hits       int
	Misses     int
	Replacer   PageReplacementAlgorithm
}

// NewTLB creates a new TLB
func NewTLB(capacity int) *TLB {
	entries := make([]*TLBEntry, capacity)
	for i := range entries {
		entries[i] = &TLBEntry{Valid: false}
	}

	return &TLB{
		Entries:  entries,
		Capacity: capacity,
		Replacer: NewLRU(capacity),
	}
}

// Lookup searches for a page in TLB
func (tlb *TLB) Lookup(pageNum int) (int, bool) {
	for _, entry := range tlb.Entries {
		if entry.Valid && entry.PageNum == pageNum {
			tlb.Hits++
			return entry.FrameNum, true
		}
	}

	tlb.Misses++
	return -1, false
}

// Insert adds a page-frame mapping to TLB
func (tlb *TLB) Insert(pageNum, frameNum, currentTime int) {
	// Find empty slot or replace using LRU
	for _, entry := range tlb.Entries {
		if !entry.Valid {
			entry.PageNum = pageNum
			entry.FrameNum = frameNum
			entry.Valid = true
			entry.LastAccess = currentTime
			return
		}
	}

	// All slots full - replace using LRU (simplified)
	oldestIdx := 0
	oldestTime := tlb.Entries[0].LastAccess

	for idx, entry := range tlb.Entries {
		if entry.LastAccess < oldestTime {
			oldestTime = entry.LastAccess
			oldestIdx = idx
		}
	}

	tlb.Entries[oldestIdx].PageNum = pageNum
	tlb.Entries[oldestIdx].FrameNum = frameNum
	tlb.Entries[oldestIdx].Valid = true
	tlb.Entries[oldestIdx].LastAccess = currentTime
}

// GetHitRate returns TLB hit rate
func (tlb *TLB) GetHitRate() float64 {
	total := tlb.Hits + tlb.Misses
	if total == 0 {
		return 0.0
	}
	return float64(tlb.Hits) / float64(total) * 100
}
