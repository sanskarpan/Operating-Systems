/*
File System Implementation
===========================

In-memory file system with different allocation methods.

Applications:
- Operating system file management
- Database storage engines
- Distributed file systems
- Virtual file systems
*/

package filesystem

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// File and Directory Structures
// =============================================================================

// FileType represents type of file system entry
type FileType int

const (
	RegularFile FileType = iota
	Directory
	SymbolicLink
)

// INode represents an index node (file metadata)
type INode struct {
	Number       int
	Type         FileType
	Size         int
	Permissions  uint16
	Owner        int
	Group        int
	CreateTime   time.Time
	ModifyTime   time.Time
	AccessTime   time.Time
	LinkCount    int
	Blocks       []int // Block numbers
	DirectBlocks []int // Direct block pointers
	IndirectBlock int  // Indirect block pointer
}

// DirectoryEntry represents an entry in a directory
type DirectoryEntry struct {
	Name   string
	INodeNum int
}

// FileDescriptor represents an open file
type FileDescriptor struct {
	FD       int
	INodeNum int
	Offset   int
	Mode     string // "r", "w", "rw"
}

// =============================================================================
// Block Management
// =============================================================================

// Block represents a disk block
type Block struct {
	Number int
	Data   []byte
	Used   bool
}

// BlockManager manages disk blocks
type BlockManager struct {
	BlockSize  int
	TotalBlocks int
	Blocks     []*Block
	FreeBlocks []int
}

// NewBlockManager creates a new block manager
func NewBlockManager(totalBlocks, blockSize int) *BlockManager {
	blocks := make([]*Block, totalBlocks)
	freeBlocks := make([]int, totalBlocks)

	for i := 0; i < totalBlocks; i++ {
		blocks[i] = &Block{
			Number: i,
			Data:   make([]byte, blockSize),
			Used:   false,
		}
		freeBlocks[i] = i
	}

	return &BlockManager{
		BlockSize:   blockSize,
		TotalBlocks: totalBlocks,
		Blocks:      blocks,
		FreeBlocks:  freeBlocks,
	}
}

// AllocateBlock allocates a free block
func (bm *BlockManager) AllocateBlock() (int, error) {
	if len(bm.FreeBlocks) == 0 {
		return -1, errors.New("no free blocks available")
	}

	blockNum := bm.FreeBlocks[0]
	bm.FreeBlocks = bm.FreeBlocks[1:]
	bm.Blocks[blockNum].Used = true

	return blockNum, nil
}

// AllocateContiguous allocates n contiguous blocks
func (bm *BlockManager) AllocateContiguous(n int) ([]int, error) {
	// Find n contiguous free blocks
	for i := 0; i <= len(bm.FreeBlocks)-n; i++ {
		isContiguous := true
		start := bm.FreeBlocks[i]

		for j := 1; j < n; j++ {
			if bm.FreeBlocks[i+j] != start+j {
				isContiguous = false
				break
			}
		}

		if isContiguous {
			blocks := make([]int, n)
			for j := 0; j < n; j++ {
				blocks[j] = bm.FreeBlocks[i+j]
				bm.Blocks[blocks[j]].Used = true
			}

			// Remove from free blocks
			bm.FreeBlocks = append(bm.FreeBlocks[:i], bm.FreeBlocks[i+n:]...)
			return blocks, nil
		}
	}

	return nil, errors.New("no contiguous blocks available")
}

// FreeBlock frees a block
func (bm *BlockManager) FreeBlock(blockNum int) {
	if blockNum >= 0 && blockNum < bm.TotalBlocks {
		bm.Blocks[blockNum].Used = false
		bm.FreeBlocks = append(bm.FreeBlocks, blockNum)
	}
}

// GetFreeBlockCount returns number of free blocks
func (bm *BlockManager) GetFreeBlockCount() int {
	return len(bm.FreeBlocks)
}

// =============================================================================
// File Allocation Methods
// =============================================================================

// AllocationMethod interface for different allocation strategies
type AllocationMethod interface {
	Allocate(size int) ([]int, error)
	Deallocate(blocks []int)
	Name() string
}

// =============================================================================
// Contiguous Allocation
// =============================================================================

// ContiguousAllocation implements contiguous file allocation
type ContiguousAllocation struct {
	BlockManager *BlockManager
}

// NewContiguousAllocation creates contiguous allocation method
func NewContiguousAllocation(bm *BlockManager) *ContiguousAllocation {
	return &ContiguousAllocation{BlockManager: bm}
}

func (ca *ContiguousAllocation) Name() string {
	return "Contiguous Allocation"
}

func (ca *ContiguousAllocation) Allocate(size int) ([]int, error) {
	numBlocks := (size + ca.BlockManager.BlockSize - 1) / ca.BlockManager.BlockSize
	return ca.BlockManager.AllocateContiguous(numBlocks)
}

func (ca *ContiguousAllocation) Deallocate(blocks []int) {
	for _, block := range blocks {
		ca.BlockManager.FreeBlock(block)
	}
}

// =============================================================================
// Linked Allocation
// =============================================================================

// LinkedAllocation implements linked file allocation
type LinkedAllocation struct {
	BlockManager *BlockManager
	NextBlock    map[int]int // Block -> Next block mapping
}

// NewLinkedAllocation creates linked allocation method
func NewLinkedAllocation(bm *BlockManager) *LinkedAllocation {
	return &LinkedAllocation{
		BlockManager: bm,
		NextBlock:    make(map[int]int),
	}
}

func (la *LinkedAllocation) Name() string {
	return "Linked Allocation"
}

func (la *LinkedAllocation) Allocate(size int) ([]int, error) {
	numBlocks := (size + la.BlockManager.BlockSize - 1) / la.BlockManager.BlockSize
	blocks := make([]int, numBlocks)

	for i := 0; i < numBlocks; i++ {
		blockNum, err := la.BlockManager.AllocateBlock()
		if err != nil {
			// Deallocate already allocated blocks
			for j := 0; j < i; j++ {
				la.BlockManager.FreeBlock(blocks[j])
			}
			return nil, err
		}
		blocks[i] = blockNum

		// Link to previous block
		if i > 0 {
			la.NextBlock[blocks[i-1]] = blockNum
		}
	}

	// Last block points to -1 (end)
	if len(blocks) > 0 {
		la.NextBlock[blocks[len(blocks)-1]] = -1
	}

	return blocks, nil
}

func (la *LinkedAllocation) Deallocate(blocks []int) {
	for _, block := range blocks {
		la.BlockManager.FreeBlock(block)
		delete(la.NextBlock, block)
	}
}

// =============================================================================
// Indexed Allocation
// =============================================================================

// IndexedAllocation implements indexed file allocation
type IndexedAllocation struct {
	BlockManager *BlockManager
	IndexBlocks  map[int][]int // Index block -> data blocks
}

// NewIndexedAllocation creates indexed allocation method
func NewIndexedAllocation(bm *BlockManager) *IndexedAllocation {
	return &IndexedAllocation{
		BlockManager: bm,
		IndexBlocks:  make(map[int][]int),
	}
}

func (ia *IndexedAllocation) Name() string {
	return "Indexed Allocation"
}

func (ia *IndexedAllocation) Allocate(size int) ([]int, error) {
	numBlocks := (size + ia.BlockManager.BlockSize - 1) / ia.BlockManager.BlockSize

	// Allocate index block
	indexBlock, err := ia.BlockManager.AllocateBlock()
	if err != nil {
		return nil, err
	}

	// Allocate data blocks
	dataBlocks := make([]int, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blockNum, err := ia.BlockManager.AllocateBlock()
		if err != nil {
			// Cleanup
			ia.BlockManager.FreeBlock(indexBlock)
			for j := 0; j < i; j++ {
				ia.BlockManager.FreeBlock(dataBlocks[j])
			}
			return nil, err
		}
		dataBlocks[i] = blockNum
	}

	ia.IndexBlocks[indexBlock] = dataBlocks
	return append([]int{indexBlock}, dataBlocks...), nil
}

func (ia *IndexedAllocation) Deallocate(blocks []int) {
	if len(blocks) == 0 {
		return
	}

	indexBlock := blocks[0]
	ia.BlockManager.FreeBlock(indexBlock)

	if dataBlocks, exists := ia.IndexBlocks[indexBlock]; exists {
		for _, block := range dataBlocks {
			ia.BlockManager.FreeBlock(block)
		}
		delete(ia.IndexBlocks, indexBlock)
	}
}

// =============================================================================
// In-Memory File System
// =============================================================================

// FileSystem represents an in-memory file system
type FileSystem struct {
	BlockManager     *BlockManager
	AllocationMethod AllocationMethod
	INodes           map[int]*INode
	OpenFiles        map[int]*FileDescriptor
	RootINode        int
	NextINodeNum     int
	NextFD           int
	CurrentDir       int
}

// NewFileSystem creates a new in-memory file system
func NewFileSystem(totalBlocks, blockSize int, allocMethod string) *FileSystem {
	bm := NewBlockManager(totalBlocks, blockSize)

	var alloc AllocationMethod
	switch allocMethod {
	case "contiguous":
		alloc = NewContiguousAllocation(bm)
	case "linked":
		alloc = NewLinkedAllocation(bm)
	case "indexed":
		alloc = NewIndexedAllocation(bm)
	default:
		alloc = NewIndexedAllocation(bm)
	}

	fs := &FileSystem{
		BlockManager:     bm,
		AllocationMethod: alloc,
		INodes:           make(map[int]*INode),
		OpenFiles:        make(map[int]*FileDescriptor),
		NextINodeNum:     1,
		NextFD:           3, // 0,1,2 reserved for stdin,stdout,stderr
	}

	// Create root directory
	rootINode := &INode{
		Number:      0,
		Type:        Directory,
		Size:        0,
		Permissions: 0755,
		CreateTime:  time.Now(),
		ModifyTime:  time.Now(),
		AccessTime:  time.Now(),
		LinkCount:   2, // . and parent
		Blocks:      make([]int, 0),
	}

	fs.INodes[0] = rootINode
	fs.RootINode = 0
	fs.CurrentDir = 0

	return fs
}

// CreateFile creates a new file
func (fs *FileSystem) CreateFile(name string, size int) (int, error) {
	// Allocate blocks
	blocks, err := fs.AllocationMethod.Allocate(size)
	if err != nil {
		return -1, err
	}

	// Create inode
	inode := &INode{
		Number:      fs.NextINodeNum,
		Type:        RegularFile,
		Size:        size,
		Permissions: 0644,
		CreateTime:  time.Now(),
		ModifyTime:  time.Now(),
		AccessTime:  time.Now(),
		LinkCount:   1,
		Blocks:      blocks,
	}

	fs.INodes[fs.NextINodeNum] = inode
	inodeNum := fs.NextINodeNum
	fs.NextINodeNum++

	// Add to current directory
	dirINode := fs.INodes[fs.CurrentDir]
	entry := DirectoryEntry{Name: name, INodeNum: inodeNum}
	fs.addDirectoryEntry(dirINode, entry)

	return inodeNum, nil
}

// DeleteFile deletes a file
func (fs *FileSystem) DeleteFile(name string) error {
	dirINode := fs.INodes[fs.CurrentDir]
	inodeNum, err := fs.findInDirectory(dirINode, name)
	if err != nil {
		return err
	}

	inode := fs.INodes[inodeNum]
	if inode.Type == Directory {
		return errors.New("cannot delete directory as file")
	}

	// Deallocate blocks
	fs.AllocationMethod.Deallocate(inode.Blocks)

	// Remove from directory
	fs.removeDirectoryEntry(dirINode, name)

	// Remove inode
	delete(fs.INodes, inodeNum)

	return nil
}

// Open opens a file and returns file descriptor
func (fs *FileSystem) Open(name string, mode string) (int, error) {
	dirINode := fs.INodes[fs.CurrentDir]
	inodeNum, err := fs.findInDirectory(dirINode, name)
	if err != nil {
		return -1, err
	}

	fd := &FileDescriptor{
		FD:       fs.NextFD,
		INodeNum: inodeNum,
		Offset:   0,
		Mode:     mode,
	}

	fs.OpenFiles[fs.NextFD] = fd
	fdNum := fs.NextFD
	fs.NextFD++

	return fdNum, nil
}

// Close closes a file descriptor
func (fs *FileSystem) Close(fd int) error {
	if _, exists := fs.OpenFiles[fd]; !exists {
		return errors.New("invalid file descriptor")
	}

	delete(fs.OpenFiles, fd)
	return nil
}

// Read reads from a file
func (fs *FileSystem) Read(fd int, size int) ([]byte, error) {
	fdesc, exists := fs.OpenFiles[fd]
	if !exists {
		return nil, errors.New("invalid file descriptor")
	}

	if fdesc.Mode != "r" && fdesc.Mode != "rw" {
		return nil, errors.New("file not open for reading")
	}

	inode := fs.INodes[fdesc.INodeNum]
	if fdesc.Offset >= inode.Size {
		return nil, errors.New("EOF")
	}

	// Read from blocks (simplified)
	readSize := size
	if fdesc.Offset+size > inode.Size {
		readSize = inode.Size - fdesc.Offset
	}

	data := make([]byte, readSize)
	// In real implementation, would read from actual blocks
	// Here we just simulate

	fdesc.Offset += readSize
	inode.AccessTime = time.Now()

	return data, nil
}

// Write writes to a file
func (fs *FileSystem) Write(fd int, data []byte) (int, error) {
	fdesc, exists := fs.OpenFiles[fd]
	if !exists {
		return 0, errors.New("invalid file descriptor")
	}

	if fdesc.Mode != "w" && fdesc.Mode != "rw" {
		return 0, errors.New("file not open for writing")
	}

	inode := fs.INodes[fdesc.INodeNum]

	// Write to blocks (simplified)
	bytesWritten := len(data)
	fdesc.Offset += bytesWritten
	if fdesc.Offset > inode.Size {
		inode.Size = fdesc.Offset
	}

	inode.ModifyTime = time.Now()

	return bytesWritten, nil
}

// MakeDirectory creates a new directory
func (fs *FileSystem) MakeDirectory(name string) error {
	// Allocate block for directory entries
	blocks, err := fs.AllocationMethod.Allocate(fs.BlockManager.BlockSize)
	if err != nil {
		return err
	}

	// Create directory inode
	inode := &INode{
		Number:      fs.NextINodeNum,
		Type:        Directory,
		Size:        0,
		Permissions: 0755,
		CreateTime:  time.Now(),
		ModifyTime:  time.Now(),
		AccessTime:  time.Now(),
		LinkCount:   2,
		Blocks:      blocks,
	}

	fs.INodes[fs.NextINodeNum] = inode
	inodeNum := fs.NextINodeNum
	fs.NextINodeNum++

	// Add to current directory
	dirINode := fs.INodes[fs.CurrentDir]
	entry := DirectoryEntry{Name: name, INodeNum: inodeNum}
	fs.addDirectoryEntry(dirINode, entry)

	return nil
}

// ChangeDirectory changes current directory
func (fs *FileSystem) ChangeDirectory(name string) error {
	if name == "/" {
		fs.CurrentDir = fs.RootINode
		return nil
	}

	if name == ".." {
		// Go to parent (simplified - would need parent tracking)
		fs.CurrentDir = fs.RootINode
		return nil
	}

	dirINode := fs.INodes[fs.CurrentDir]
	inodeNum, err := fs.findInDirectory(dirINode, name)
	if err != nil {
		return err
	}

	inode := fs.INodes[inodeNum]
	if inode.Type != Directory {
		return errors.New("not a directory")
	}

	fs.CurrentDir = inodeNum
	return nil
}

// ListDirectory lists contents of current directory
func (fs *FileSystem) ListDirectory() []string {
	dirINode := fs.INodes[fs.CurrentDir]
	entries := fs.getDirectoryEntries(dirINode)

	result := make([]string, len(entries))
	for i, entry := range entries {
		inode := fs.INodes[entry.INodeNum]
		typeStr := "f"
		if inode.Type == Directory {
			typeStr = "d"
		}
		result[i] = fmt.Sprintf("%s %s %d bytes", typeStr, entry.Name, inode.Size)
	}

	return result
}

// Helper functions for directory management
func (fs *FileSystem) addDirectoryEntry(dirINode *INode, entry DirectoryEntry) {
	// In real implementation, would write to blocks
	// Here we use a simplified approach
	dirINode.Size += len(entry.Name) + 8 // Simplified
}

func (fs *FileSystem) removeDirectoryEntry(dirINode *INode, name string) {
	// Simplified
	dirINode.Size -= len(name) + 8
}

func (fs *FileSystem) findInDirectory(dirINode *INode, name string) (int, error) {
	// In real implementation, would search through blocks
	// Here we use a simplified lookup (would need proper directory block parsing)
	return -1, errors.New("file not found")
}

func (fs *FileSystem) getDirectoryEntries(dirINode *INode) []DirectoryEntry {
	// Simplified - would parse directory blocks
	return []DirectoryEntry{}
}

// =============================================================================
// Free Space Management
// =============================================================================

// FreeSpaceManager manages free disk space
type FreeSpaceManager interface {
	AllocateBlock() (int, error)
	FreeBlock(blockNum int)
	IsFree(blockNum int) bool
}

// BitVector implements free space management using bit vector
type BitVector struct {
	Size    int
	Bitmap  []bool
	FreeCount int
}

// NewBitVector creates a bit vector for free space management
func NewBitVector(size int) *BitVector {
	return &BitVector{
		Size:      size,
		Bitmap:    make([]bool, size),
		FreeCount: size,
	}
}

func (bv *BitVector) AllocateBlock() (int, error) {
	for i, free := range bv.Bitmap {
		if !free {
			bv.Bitmap[i] = true
			bv.FreeCount--
			return i, nil
		}
	}
	return -1, errors.New("no free blocks")
}

func (bv *BitVector) FreeBlock(blockNum int) {
	if blockNum >= 0 && blockNum < bv.Size && bv.Bitmap[blockNum] {
		bv.Bitmap[blockNum] = false
		bv.FreeCount++
	}
}

func (bv *BitVector) IsFree(blockNum int) bool {
	if blockNum >= 0 && blockNum < bv.Size {
		return !bv.Bitmap[blockNum]
	}
	return false
}

// LinkedList implements free space management using linked list
type FreeBlockList struct {
	Head *FreeBlockNode
	Size int
}

type FreeBlockNode struct {
	BlockNum int
	Next     *FreeBlockNode
}

// NewFreeBlockList creates a linked list for free space management
func NewFreeBlockList(totalBlocks int) *FreeBlockList {
	fbl := &FreeBlockList{Size: totalBlocks}

	// Initialize all blocks as free
	var prev *FreeBlockNode
	for i := totalBlocks - 1; i >= 0; i-- {
		node := &FreeBlockNode{BlockNum: i, Next: prev}
		prev = node
	}
	fbl.Head = prev

	return fbl
}

func (fbl *FreeBlockList) AllocateBlock() (int, error) {
	if fbl.Head == nil {
		return -1, errors.New("no free blocks")
	}

	blockNum := fbl.Head.BlockNum
	fbl.Head = fbl.Head.Next
	fbl.Size--

	return blockNum, nil
}

func (fbl *FreeBlockList) FreeBlock(blockNum int) {
	node := &FreeBlockNode{BlockNum: blockNum, Next: fbl.Head}
	fbl.Head = node
	fbl.Size++
}

// =============================================================================
// File System Operations
// =============================================================================

// Stat returns file information
func (fs *FileSystem) Stat(name string) (*INode, error) {
	dirINode := fs.INodes[fs.CurrentDir]
	inodeNum, err := fs.findInDirectory(dirINode, name)
	if err != nil {
		return nil, err
	}

	return fs.INodes[inodeNum], nil
}

// GetDiskUsage returns disk usage statistics
func (fs *FileSystem) GetDiskUsage() (used, free, total int) {
	total = fs.BlockManager.TotalBlocks
	free = fs.BlockManager.GetFreeBlockCount()
	used = total - free
	return
}

// Path represents a file system path
type Path struct {
	Components []string
	IsAbsolute bool
}

// ParsePath parses a path string
func ParsePath(pathStr string) *Path {
	isAbsolute := strings.HasPrefix(pathStr, "/")
	components := strings.Split(strings.Trim(pathStr, "/"), "/")

	// Remove empty components
	filtered := make([]string, 0)
	for _, comp := range components {
		if comp != "" {
			filtered = append(filtered, comp)
		}
	}

	return &Path{
		Components: filtered,
		IsAbsolute: isAbsolute,
	}
}

// String converts path back to string
func (p *Path) String() string {
	if p.IsAbsolute {
		return "/" + strings.Join(p.Components, "/")
	}
	return strings.Join(p.Components, "/")
}
