package filesystem

// =============================================================================
// FS-001: Write-Ahead Log (WAL) Journaling
// =============================================================================
//
// A WAL guarantees atomicity: every change is logged before it is applied.
// On recovery, committed entries are replayed; uncommitted entries are ignored.

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// WALOpType distinguishes Put from Delete operations.
type WALOpType uint8

const (
	WALPut    WALOpType = 1
	WALDelete WALOpType = 2
)

// WALEntry is one record in the log.
type WALEntry struct {
	LSN       int64
	Op        WALOpType
	Key       string
	Value     []byte
	Committed bool
}

// WAL is an append-only journal stored in memory.
type WAL struct {
	mu      sync.Mutex
	entries []*WALEntry
	nextLSN int64
}

// NewWAL returns an empty write-ahead log.
func NewWAL() *WAL { return &WAL{} }

// Append writes an uncommitted log entry and returns its LSN.
func (w *WAL) Append(op WALOpType, key string, value []byte) int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	lsn := w.nextLSN
	w.nextLSN++
	v := make([]byte, len(value))
	copy(v, value)
	w.entries = append(w.entries, &WALEntry{LSN: lsn, Op: op, Key: key, Value: v})
	return lsn
}

// Commit marks the entry at lsn as durable.
func (w *WAL) Commit(lsn int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range w.entries {
		if e.LSN == lsn {
			e.Committed = true
			return nil
		}
	}
	return fmt.Errorf("WAL: LSN %d not found", lsn)
}

// Entries returns a snapshot of all log entries (read-only view).
func (w *WAL) Entries() []*WALEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*WALEntry, len(w.entries))
	copy(out, w.entries)
	return out
}

// Len returns the number of log records.
func (w *WAL) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.entries)
}

// WALStore is a simple key-value store that uses WAL for crash recovery.
type WALStore struct {
	wal   *WAL
	mu    sync.RWMutex
	store map[string][]byte
}

// NewWALStore creates a WAL-backed key-value store.
func NewWALStore() *WALStore {
	return &WALStore{
		wal:   NewWAL(),
		store: make(map[string][]byte),
	}
}

// Put writes a key-value pair, logging first.
func (s *WALStore) Put(key string, value []byte) {
	lsn := s.wal.Append(WALPut, key, value)
	s.mu.Lock()
	v := make([]byte, len(value))
	copy(v, value)
	s.store[key] = v
	s.mu.Unlock()
	s.wal.Commit(lsn) //nolint:errcheck
}

// Delete removes a key, logging first.
func (s *WALStore) Delete(key string) {
	lsn := s.wal.Append(WALDelete, key, nil)
	s.mu.Lock()
	delete(s.store, key)
	s.mu.Unlock()
	s.wal.Commit(lsn) //nolint:errcheck
}

// Get retrieves the value for key.
func (s *WALStore) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[key]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, true
}

// Recover replays committed WAL entries into a fresh store.
// Simulates crash recovery: only committed entries survive.
func (s *WALStore) Recover() *WALStore {
	fresh := &WALStore{
		wal:   s.wal,
		store: make(map[string][]byte),
	}
	for _, e := range s.wal.Entries() {
		if !e.Committed {
			continue
		}
		switch e.Op {
		case WALPut:
			v := make([]byte, len(e.Value))
			copy(v, e.Value)
			fresh.store[e.Key] = v
		case WALDelete:
			delete(fresh.store, e.Key)
		}
	}
	return fresh
}

// WAL returns the underlying log (for inspection in tests).
func (s *WALStore) WALLog() *WAL { return s.wal }

// =============================================================================
// FS-002: Hard Links & Reference Counting
// =============================================================================
//
// Each inode has a reference count. A hard link creates a new directory entry
// pointing at the same inode and bumps the count. Unlink decrements it; the
// data is released only when the count reaches zero.

// HardLinkInode is an inode with reference-counted data.
type HardLinkInode struct {
	ID       int
	refCount int32 // accessed atomically
	data     []byte
	mu       sync.RWMutex
}

// RefCount returns the current reference count.
func (i *HardLinkInode) RefCount() int { return int(atomic.LoadInt32(&i.refCount)) }

// Data returns a copy of the inode's data (nil if deleted).
func (i *HardLinkInode) Data() []byte {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.data == nil {
		return nil
	}
	out := make([]byte, len(i.data))
	copy(out, i.data)
	return out
}

// HardLinkFS is a minimal filesystem that demonstrates hard-link semantics.
type HardLinkFS struct {
	mu      sync.Mutex
	inodes  map[int]*HardLinkInode
	dirents map[string]int // name → inode ID
	nextID  int
}

// NewHardLinkFS creates an empty filesystem.
func NewHardLinkFS() *HardLinkFS {
	return &HardLinkFS{
		inodes:  make(map[int]*HardLinkInode),
		dirents: make(map[string]int),
	}
}

// Create makes a new file with the given name and initial data.
// Returns the inode ID.
func (fs *HardLinkFS) Create(name string, data []byte) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, exists := fs.dirents[name]; exists {
		return 0, fmt.Errorf("hardlinkfs: %q already exists", name)
	}
	id := fs.nextID
	fs.nextID++
	d := make([]byte, len(data))
	copy(d, data)
	ino := &HardLinkInode{ID: id, data: d}
	atomic.StoreInt32(&ino.refCount, 1)
	fs.inodes[id] = ino
	fs.dirents[name] = id
	return id, nil
}

// Link creates a hard link: newName → same inode as oldName.
func (fs *HardLinkFS) Link(oldName, newName string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	id, ok := fs.dirents[oldName]
	if !ok {
		return fmt.Errorf("hardlinkfs: %q not found", oldName)
	}
	if _, exists := fs.dirents[newName]; exists {
		return fmt.Errorf("hardlinkfs: %q already exists", newName)
	}
	atomic.AddInt32(&fs.inodes[id].refCount, 1)
	fs.dirents[newName] = id
	return nil
}

// Unlink removes a directory entry. If refcount drops to 0, data is released.
func (fs *HardLinkFS) Unlink(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	id, ok := fs.dirents[name]
	if !ok {
		return fmt.Errorf("hardlinkfs: %q not found", name)
	}
	delete(fs.dirents, name)
	ino := fs.inodes[id]
	remaining := atomic.AddInt32(&ino.refCount, -1)
	if remaining == 0 {
		ino.mu.Lock()
		ino.data = nil
		ino.mu.Unlock()
		delete(fs.inodes, id)
	}
	return nil
}

// Read returns the data for the named file.
func (fs *HardLinkFS) Read(name string) ([]byte, error) {
	fs.mu.Lock()
	id, ok := fs.dirents[name]
	var ino *HardLinkInode
	if ok {
		ino = fs.inodes[id]
	}
	fs.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("hardlinkfs: %q not found", name)
	}
	data := ino.Data()
	if data == nil {
		return nil, fmt.Errorf("hardlinkfs: inode %d deleted", id)
	}
	return data, nil
}

// InodeFor returns the inode ID for a name.
func (fs *HardLinkFS) InodeFor(name string) (int, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	id, ok := fs.dirents[name]
	return id, ok
}

// RefCountFor returns the reference count of the inode behind name.
func (fs *HardLinkFS) RefCountFor(name string) (int, error) {
	fs.mu.Lock()
	id, ok := fs.dirents[name]
	var ino *HardLinkInode
	if ok {
		ino = fs.inodes[id]
	}
	fs.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("hardlinkfs: %q not found", name)
	}
	return ino.RefCount(), nil
}

// =============================================================================
// FS-003: Buffer Cache (LRU)
// =============================================================================
//
// The buffer cache sits in front of the block manager. Reads are served from
// the cache; writes go to the cache and mark the block dirty. Flush() writes
// dirty blocks back. HitRate() tracks the cache effectiveness.

type cacheBlock struct {
	blockNum int
	data     []byte
	dirty    bool
	// LRU linkage
	prev, next *cacheBlock
}

// BufferCache is an LRU block cache layered over a BlockManager.
type BufferCache struct {
	bm       *BlockManager
	capacity int
	mu       sync.Mutex

	table map[int]*cacheBlock // blockNum → node

	// Doubly-linked list: head = most-recently used, tail = LRU victim
	head, tail *cacheBlock

	hits, misses int64
	FlushCount   int64
}

// NewBufferCache creates a buffer cache of the given capacity over bm.
func NewBufferCache(bm *BlockManager, capacity int) *BufferCache {
	return &BufferCache{
		bm:       bm,
		capacity: capacity,
		table:    make(map[int]*cacheBlock),
	}
}

// Read returns a copy of block data, loading from BlockManager on a miss.
func (c *BufferCache) Read(blockNum int) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if node, ok := c.table[blockNum]; ok {
		c.hits++
		c.moveToFront(node)
		out := make([]byte, len(node.data))
		copy(out, node.data)
		return out, nil
	}
	// Cache miss: load from block manager.
	c.misses++
	if blockNum < 0 || blockNum >= c.bm.TotalBlocks {
		return nil, fmt.Errorf("buffer cache: block %d out of range", blockNum)
	}
	raw := c.bm.Blocks[blockNum].Data
	node := c.insertFront(blockNum, raw)
	out := make([]byte, len(node.data))
	copy(out, node.data)
	return out, nil
}

// Write updates block data in the cache and marks it dirty.
func (c *BufferCache) Write(blockNum int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if blockNum < 0 || blockNum >= c.bm.TotalBlocks {
		return fmt.Errorf("buffer cache: block %d out of range", blockNum)
	}
	if node, ok := c.table[blockNum]; ok {
		copy(node.data, data)
		if len(data) > len(node.data) {
			node.data = append(node.data[:0], data...)
		}
		node.dirty = true
		c.moveToFront(node)
		return nil
	}
	node := c.insertFront(blockNum, data)
	node.dirty = true
	return nil
}

// Flush writes all dirty blocks back to the BlockManager.
func (c *BufferCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, node := range c.table {
		if node.dirty {
			dst := c.bm.Blocks[node.blockNum].Data
			copy(dst, node.data)
			node.dirty = false
			atomic.AddInt64(&c.FlushCount, 1)
		}
	}
}

// HitRate returns the fraction of reads served from cache.
func (c *BufferCache) HitRate() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}

// Size returns the number of blocks currently in the cache.
func (c *BufferCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.table)
}

// insertFront adds a new node (or replaces if full), returns the node.
func (c *BufferCache) insertFront(blockNum int, data []byte) *cacheBlock {
	if len(c.table) >= c.capacity {
		c.evict()
	}
	d := make([]byte, len(data))
	copy(d, data)
	node := &cacheBlock{blockNum: blockNum, data: d}
	c.table[blockNum] = node
	// Prepend to list.
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}
	return node
}

// moveToFront promotes a node to the front (most-recently used).
func (c *BufferCache) moveToFront(node *cacheBlock) {
	if c.head == node {
		return
	}
	// Detach.
	if node.prev != nil {
		node.prev.next = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}
	if c.tail == node {
		c.tail = node.prev
	}
	// Prepend.
	node.prev = nil
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
}

// evict removes the least-recently-used (tail) block.
func (c *BufferCache) evict() {
	if c.tail == nil {
		return
	}
	victim := c.tail
	if victim.dirty {
		// Write back before eviction.
		copy(c.bm.Blocks[victim.blockNum].Data, victim.data)
		atomic.AddInt64(&c.FlushCount, 1)
	}
	if victim.prev != nil {
		victim.prev.next = nil
	} else {
		c.head = nil
	}
	c.tail = victim.prev
	delete(c.table, victim.blockNum)
}

// =============================================================================
// FS-004: Copy-on-Write (CoW)
// =============================================================================
//
// Pages are shared read-only across processes after a fork. The first write by
// any process triggers a private copy of the page for that process. Snapshots
// capture an immutable view of all pages at a point in time.

// cowPage holds the data for one virtual page.
type cowPage struct {
	mu      sync.RWMutex
	data    []byte
	owners  map[int]struct{} // process IDs sharing this physical page
	private map[int][]byte   // per-process private copies
}

// CoWManager manages copy-on-write page mappings.
type CoWManager struct {
	mu    sync.Mutex
	pages map[int]*cowPage // virtual page number → page

	WriteFaults int64 // number of CoW faults triggered
}

// NewCoWManager creates an empty CoW manager.
func NewCoWManager() *CoWManager {
	return &CoWManager{pages: make(map[int]*cowPage)}
}

// AllocatePage creates a new page owned by pid with the given data.
func (m *CoWManager) AllocatePage(pid, pageNum int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.pages[pageNum]; exists {
		return fmt.Errorf("cow: page %d already allocated", pageNum)
	}
	d := make([]byte, len(data))
	copy(d, data)
	p := &cowPage{
		data:    d,
		owners:  map[int]struct{}{pid: {}},
		private: make(map[int][]byte),
	}
	m.pages[pageNum] = p
	return nil
}

// Fork shares all of parent's pages with child as read-only.
func (m *CoWManager) Fork(parentPID, childPID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.pages {
		p.mu.Lock()
		if _, owned := p.owners[parentPID]; owned {
			p.owners[childPID] = struct{}{}
		}
		p.mu.Unlock()
	}
}

// Read returns the data for pageNum as seen by pid.
func (m *CoWManager) Read(pid, pageNum int) ([]byte, error) {
	m.mu.Lock()
	p, ok := m.pages[pageNum]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cow: page %d not found", pageNum)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	// If pid has a private copy, return that.
	if priv, ok := p.private[pid]; ok {
		out := make([]byte, len(priv))
		copy(out, priv)
		return out, nil
	}
	// Otherwise return shared data.
	out := make([]byte, len(p.data))
	copy(out, p.data)
	return out, nil
}

// Write performs a CoW: if the page is shared, make a private copy first.
func (m *CoWManager) Write(pid, pageNum int, data []byte) error {
	m.mu.Lock()
	p, ok := m.pages[pageNum]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("cow: page %d not found", pageNum)
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if pid is the sole owner and has no other sharing processes.
	sharedCount := len(p.owners)
	_, hasPrivate := p.private[pid]

	if sharedCount > 1 && !hasPrivate {
		// CoW fault: create private copy for pid.
		atomic.AddInt64(&m.WriteFaults, 1)
		priv := make([]byte, len(p.data))
		copy(priv, p.data)
		copy(priv, data) // apply the write to the private copy
		p.private[pid] = priv
		return nil
	}

	// Sole owner or already has private copy — write directly.
	if hasPrivate {
		copy(p.private[pid], data)
	} else {
		copy(p.data, data)
	}
	return nil
}

// Snapshot captures an immutable point-in-time view of all pages for pid.
// Returns map[pageNum]data.
func (m *CoWManager) Snapshot(pid int) map[int][]byte {
	m.mu.Lock()
	pageNums := make([]int, 0, len(m.pages))
	for n := range m.pages {
		pageNums = append(pageNums, n)
	}
	m.mu.Unlock()

	snap := make(map[int][]byte, len(pageNums))
	for _, n := range pageNums {
		if data, err := m.Read(pid, n); err == nil {
			snap[n] = data
		}
	}
	return snap
}

// PageCount returns the number of allocated pages.
func (m *CoWManager) PageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pages)
}

// sentinel to satisfy "errors" import usage
var _ = errors.New
