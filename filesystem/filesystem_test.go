package filesystem

import (
	"testing"
)

// =============================================================================
// BlockManager tests
// =============================================================================

func TestBlockManagerAllocate(t *testing.T) {
	bm := NewBlockManager(10, 512)

	if bm.GetFreeBlockCount() != 10 {
		t.Errorf("Expected 10 free blocks, got %d", bm.GetFreeBlockCount())
	}

	blockNum, err := bm.AllocateBlock()
	if err != nil {
		t.Fatalf("AllocateBlock failed: %v", err)
	}

	if blockNum < 0 || blockNum >= 10 {
		t.Errorf("Unexpected block number: %d", blockNum)
	}

	if bm.GetFreeBlockCount() != 9 {
		t.Errorf("Expected 9 free blocks after allocation, got %d", bm.GetFreeBlockCount())
	}
}

func TestBlockManagerFree(t *testing.T) {
	bm := NewBlockManager(5, 512)

	blockNum, err := bm.AllocateBlock()
	if err != nil {
		t.Fatalf("AllocateBlock failed: %v", err)
	}

	bm.FreeBlock(blockNum)

	if bm.GetFreeBlockCount() != 5 {
		t.Errorf("Expected 5 free blocks after free, got %d", bm.GetFreeBlockCount())
	}
}

func TestBlockManagerExhausted(t *testing.T) {
	bm := NewBlockManager(2, 512)

	bm.AllocateBlock()
	bm.AllocateBlock()

	_, err := bm.AllocateBlock()
	if err == nil {
		t.Error("AllocateBlock should fail when no free blocks remain")
	}
}

func TestBlockManagerAllocateContiguous(t *testing.T) {
	bm := NewBlockManager(10, 512)

	blocks, err := bm.AllocateContiguous(3)
	if err != nil {
		t.Fatalf("AllocateContiguous failed: %v", err)
	}

	if len(blocks) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(blocks))
	}

	// Verify they are contiguous
	for i := 1; i < len(blocks); i++ {
		if blocks[i] != blocks[i-1]+1 {
			t.Errorf("Blocks are not contiguous: %v", blocks)
		}
	}

	if bm.GetFreeBlockCount() != 7 {
		t.Errorf("Expected 7 free blocks, got %d", bm.GetFreeBlockCount())
	}
}

func TestBlockManagerAllocateContiguousInsufficient(t *testing.T) {
	bm := NewBlockManager(3, 512)

	_, err := bm.AllocateContiguous(5)
	if err == nil {
		t.Error("AllocateContiguous should fail when not enough contiguous blocks exist")
	}
}

// =============================================================================
// Allocation method tests
// =============================================================================

func TestContiguousAllocation(t *testing.T) {
	bm := NewBlockManager(20, 512)
	ca := NewContiguousAllocation(bm)

	if ca.Name() != "Contiguous Allocation" {
		t.Errorf("Unexpected name: %s", ca.Name())
	}

	// Allocate 1024 bytes = 2 blocks of 512
	blocks, err := ca.Allocate(1024)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if len(blocks) != 2 {
		t.Errorf("Expected 2 blocks for 1024 bytes, got %d", len(blocks))
	}

	before := bm.GetFreeBlockCount()
	ca.Deallocate(blocks)
	after := bm.GetFreeBlockCount()

	if after-before != 2 {
		t.Errorf("Deallocate did not free the correct number of blocks")
	}
}

func TestLinkedAllocation(t *testing.T) {
	bm := NewBlockManager(20, 512)
	la := NewLinkedAllocation(bm)

	if la.Name() != "Linked Allocation" {
		t.Errorf("Unexpected name: %s", la.Name())
	}

	blocks, err := la.Allocate(1536) // 3 blocks
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if len(blocks) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(blocks))
	}

	// Verify linked list chain
	for i := 0; i < len(blocks)-1; i++ {
		next, exists := la.NextBlock[blocks[i]]
		if !exists {
			t.Errorf("Block %d has no next pointer", blocks[i])
		}
		if next != blocks[i+1] {
			t.Errorf("Block %d links to %d, expected %d", blocks[i], next, blocks[i+1])
		}
	}

	// Last block should point to -1
	if la.NextBlock[blocks[len(blocks)-1]] != -1 {
		t.Error("Last block should point to -1")
	}

	before := bm.GetFreeBlockCount()
	la.Deallocate(blocks)
	after := bm.GetFreeBlockCount()

	if after-before != 3 {
		t.Errorf("Deallocate did not free the correct number of blocks")
	}
}

func TestIndexedAllocation(t *testing.T) {
	bm := NewBlockManager(20, 512)
	ia := NewIndexedAllocation(bm)

	if ia.Name() != "Indexed Allocation" {
		t.Errorf("Unexpected name: %s", ia.Name())
	}

	// 1 index block + 2 data blocks for 1024 bytes
	blocks, err := ia.Allocate(1024)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	// blocks[0] is the index block; rest are data blocks
	if len(blocks) < 2 {
		t.Errorf("Expected at least index+data blocks, got %d", len(blocks))
	}

	indexBlock := blocks[0]
	if _, exists := ia.IndexBlocks[indexBlock]; !exists {
		t.Error("Index block not recorded")
	}

	before := bm.GetFreeBlockCount()
	ia.Deallocate(blocks)
	after := bm.GetFreeBlockCount()

	if after <= before {
		t.Error("Deallocate did not free blocks")
	}
}

func TestIndexedAllocationDeallocateEmpty(t *testing.T) {
	bm := NewBlockManager(10, 512)
	ia := NewIndexedAllocation(bm)

	// Should not panic on empty slice
	ia.Deallocate([]int{})
}

// =============================================================================
// FileSystem tests
// =============================================================================

func TestNewFileSystem(t *testing.T) {
	for _, method := range []string{"contiguous", "linked", "indexed", "unknown"} {
		fs := NewFileSystem(50, 512, method)
		if fs == nil {
			t.Fatalf("NewFileSystem returned nil for method %q", method)
		}
		if fs.RootINode != 0 {
			t.Errorf("Expected root inode 0, got %d", fs.RootINode)
		}
		if fs.CurrentDir != 0 {
			t.Errorf("Expected current dir 0, got %d", fs.CurrentDir)
		}
	}
}

func TestFileSystemCreateFile(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	inodeNum, err := fs.CreateFile("test.txt", 512)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	if inodeNum <= 0 {
		t.Errorf("Expected positive inode number, got %d", inodeNum)
	}

	inode, exists := fs.INodes[inodeNum]
	if !exists {
		t.Fatal("INode not found after CreateFile")
	}

	if inode.Type != RegularFile {
		t.Error("Expected RegularFile type")
	}

	if inode.Size != 512 {
		t.Errorf("Expected size 512, got %d", inode.Size)
	}
}

func TestFileSystemCreateMultipleFiles(t *testing.T) {
	fs := NewFileSystem(100, 512, "linked")

	for i := 0; i < 5; i++ {
		_, err := fs.CreateFile("file", 256)
		if err != nil {
			t.Fatalf("CreateFile %d failed: %v", i, err)
		}
	}

	// 1 root inode + 5 file inodes
	if len(fs.INodes) != 6 {
		t.Errorf("Expected 6 inodes, got %d", len(fs.INodes))
	}
}

func TestFileSystemMakeDirectory(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	err := fs.MakeDirectory("subdir")
	if err != nil {
		t.Fatalf("MakeDirectory failed: %v", err)
	}

	// Find the new directory inode
	var dirInode *INode
	for num, inode := range fs.INodes {
		if num != 0 && inode.Type == Directory {
			dirInode = inode
			break
		}
	}

	if dirInode == nil {
		t.Fatal("Directory inode not found")
	}

	if dirInode.Permissions != 0755 {
		t.Errorf("Expected permissions 0755, got %o", dirInode.Permissions)
	}
}

func TestFileSystemGetDiskUsage(t *testing.T) {
	fs := NewFileSystem(20, 512, "contiguous")

	used, free, total := fs.GetDiskUsage()

	if total != 20 {
		t.Errorf("Expected total 20, got %d", total)
	}

	if used+free != total {
		t.Errorf("used (%d) + free (%d) != total (%d)", used, free, total)
	}

	// Create a file; disk usage should change
	fs.CreateFile("test.txt", 512)
	usedAfter, freeAfter, _ := fs.GetDiskUsage()

	if usedAfter <= used {
		t.Errorf("Used blocks should increase after file creation (before=%d, after=%d)", used, usedAfter)
	}

	if freeAfter >= free {
		t.Errorf("Free blocks should decrease after file creation (before=%d, after=%d)", free, freeAfter)
	}
}

func TestFileSystemChangeDirectoryRoot(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")
	fs.MakeDirectory("subdir")

	// Verify we can navigate to root
	err := fs.ChangeDirectory("/")
	if err != nil {
		t.Fatalf("ChangeDirectory(\"/\") failed: %v", err)
	}

	if fs.CurrentDir != fs.RootINode {
		t.Error("CurrentDir should be root after ChangeDirectory(\"/\")")
	}
}

func TestFileSystemChangeDirectoryParent(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	err := fs.ChangeDirectory("..")
	if err != nil {
		t.Fatalf("ChangeDirectory(\"..\") failed: %v", err)
	}

	if fs.CurrentDir != fs.RootINode {
		t.Error("CurrentDir should be root after ChangeDirectory(\"..\")")
	}
}

func TestFileSystemListDirectoryEmpty(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	entries := fs.ListDirectory()
	// getDirectoryEntries is a stub that returns empty list
	if entries == nil {
		t.Error("ListDirectory should return a non-nil slice")
	}
}

func TestFileSystemOpenNonExistentFile(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	_, err := fs.Open("nonexistent.txt", "r")
	if err == nil {
		t.Error("Open of nonexistent file should return an error")
	}
}

func TestFileSystemDeleteNonExistentFile(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	err := fs.DeleteFile("nonexistent.txt")
	if err == nil {
		t.Error("DeleteFile of nonexistent file should return an error")
	}
}

func TestFileSystemStatNonExistentFile(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	_, err := fs.Stat("nonexistent.txt")
	if err == nil {
		t.Error("Stat of nonexistent file should return an error")
	}
}

func TestFileSystemReadWrite(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	// CreateFile returns an inode; manually open it by inserting a descriptor
	inodeNum, err := fs.CreateFile("test.txt", 1024)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	// Manually register an open file descriptor for reading
	fd := 10
	fs.OpenFiles[fd] = &FileDescriptor{FD: fd, INodeNum: inodeNum, Offset: 0, Mode: "rw"}

	// Write some data
	written, err := fs.Write(fd, []byte("hello world"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if written != 11 {
		t.Errorf("Expected 11 bytes written, got %d", written)
	}

	// Reset offset to read back
	fs.OpenFiles[fd].Offset = 0

	data, err := fs.Read(fd, 5)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(data) != 5 {
		t.Errorf("Expected 5 bytes read, got %d", len(data))
	}

	// Close
	err = fs.Close(fd)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Second close should fail
	err = fs.Close(fd)
	if err == nil {
		t.Error("Second Close should return an error")
	}
}

func TestFileSystemReadInvalidMode(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")
	inodeNum, _ := fs.CreateFile("test.txt", 512)
	fd := 10
	fs.OpenFiles[fd] = &FileDescriptor{FD: fd, INodeNum: inodeNum, Offset: 0, Mode: "w"}

	_, err := fs.Read(fd, 10)
	if err == nil {
		t.Error("Read on write-only fd should return an error")
	}
}

func TestFileSystemWriteInvalidMode(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")
	inodeNum, _ := fs.CreateFile("test.txt", 512)
	fd := 10
	fs.OpenFiles[fd] = &FileDescriptor{FD: fd, INodeNum: inodeNum, Offset: 0, Mode: "r"}

	_, err := fs.Write(fd, []byte("data"))
	if err == nil {
		t.Error("Write on read-only fd should return an error")
	}
}

func TestFileSystemReadEOF(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")
	inodeNum, _ := fs.CreateFile("test.txt", 10)
	fd := 10
	fs.OpenFiles[fd] = &FileDescriptor{FD: fd, INodeNum: inodeNum, Offset: 10, Mode: "r"}

	_, err := fs.Read(fd, 5)
	if err == nil {
		t.Error("Read at EOF should return an error")
	}
}

func TestFileSystemInvalidFD(t *testing.T) {
	fs := NewFileSystem(50, 512, "indexed")

	_, err := fs.Read(999, 10)
	if err == nil {
		t.Error("Read with invalid FD should return an error")
	}

	_, err = fs.Write(999, []byte("data"))
	if err == nil {
		t.Error("Write with invalid FD should return an error")
	}

	err = fs.Close(999)
	if err == nil {
		t.Error("Close with invalid FD should return an error")
	}
}

// =============================================================================
// BitVector tests
// =============================================================================

func TestBitVectorAllocate(t *testing.T) {
	bv := NewBitVector(8)

	if bv.FreeCount != 8 {
		t.Errorf("Expected 8 free blocks, got %d", bv.FreeCount)
	}

	block, err := bv.AllocateBlock()
	if err != nil {
		t.Fatalf("AllocateBlock failed: %v", err)
	}

	if !bv.Bitmap[block] {
		t.Error("Allocated block should be marked used in bitmap")
	}

	if bv.FreeCount != 7 {
		t.Errorf("Expected 7 free blocks, got %d", bv.FreeCount)
	}
}

func TestBitVectorFree(t *testing.T) {
	bv := NewBitVector(4)

	block, _ := bv.AllocateBlock()
	bv.FreeBlock(block)

	if bv.FreeCount != 4 {
		t.Errorf("Expected 4 free blocks after free, got %d", bv.FreeCount)
	}

	if !bv.IsFree(block) {
		t.Error("Block should be free after FreeBlock")
	}
}

func TestBitVectorIsFree(t *testing.T) {
	bv := NewBitVector(4)

	if !bv.IsFree(0) {
		t.Error("Block 0 should be free initially")
	}

	bv.AllocateBlock()

	if bv.IsFree(0) {
		t.Error("Block 0 should be used after allocation")
	}

	if bv.IsFree(10) {
		t.Error("Out-of-range block should not be free")
	}
}

func TestBitVectorExhausted(t *testing.T) {
	bv := NewBitVector(2)

	bv.AllocateBlock()
	bv.AllocateBlock()

	_, err := bv.AllocateBlock()
	if err == nil {
		t.Error("AllocateBlock should fail when no free blocks remain")
	}
}

// =============================================================================
// FreeBlockList tests
// =============================================================================

func TestFreeBlockListAllocate(t *testing.T) {
	fbl := NewFreeBlockList(5)

	block, err := fbl.AllocateBlock()
	if err != nil {
		t.Fatalf("AllocateBlock failed: %v", err)
	}

	if block < 0 {
		t.Errorf("Expected non-negative block number, got %d", block)
	}

	if fbl.Size != 4 {
		t.Errorf("Expected size 4, got %d", fbl.Size)
	}
}

func TestFreeBlockListFree(t *testing.T) {
	fbl := NewFreeBlockList(3)

	block, _ := fbl.AllocateBlock()
	fbl.FreeBlock(block)

	if fbl.Size != 3 {
		t.Errorf("Expected size 3 after free, got %d", fbl.Size)
	}
}

func TestFreeBlockListExhausted(t *testing.T) {
	fbl := NewFreeBlockList(2)

	fbl.AllocateBlock()
	fbl.AllocateBlock()

	_, err := fbl.AllocateBlock()
	if err == nil {
		t.Error("AllocateBlock should fail when list is empty")
	}
}

// =============================================================================
// ParsePath tests
// =============================================================================

func TestParsePath(t *testing.T) {
	p := ParsePath("/home/user/file.txt")

	if !p.IsAbsolute {
		t.Error("Expected absolute path")
	}

	if len(p.Components) != 3 {
		t.Errorf("Expected 3 components, got %d: %v", len(p.Components), p.Components)
	}

	if p.Components[0] != "home" || p.Components[1] != "user" || p.Components[2] != "file.txt" {
		t.Errorf("Unexpected components: %v", p.Components)
	}
}

func TestParsePathRelative(t *testing.T) {
	p := ParsePath("docs/readme.md")

	if p.IsAbsolute {
		t.Error("Expected relative path")
	}

	if len(p.Components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(p.Components))
	}
}

func TestParsePathRoot(t *testing.T) {
	p := ParsePath("/")

	if !p.IsAbsolute {
		t.Error("Expected absolute path")
	}

	if len(p.Components) != 0 {
		t.Errorf("Expected 0 components for root, got %d", len(p.Components))
	}
}

func TestPathString(t *testing.T) {
	p := ParsePath("/home/user/file.txt")
	s := p.String()

	if s != "/home/user/file.txt" {
		t.Errorf("Expected '/home/user/file.txt', got '%s'", s)
	}
}

func TestPathStringRelative(t *testing.T) {
	p := ParsePath("docs/readme.md")
	s := p.String()

	if s != "docs/readme.md" {
		t.Errorf("Expected 'docs/readme.md', got '%s'", s)
	}
}
