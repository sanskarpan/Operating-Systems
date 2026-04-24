package filesystem

import (
	"sync"
	"testing"
)

// =============================================================================
// FS-001: WAL Journaling Tests
// =============================================================================

func TestWAL_AppendCommit(t *testing.T) {
	w := NewWAL()
	lsn := w.Append(WALPut, "k", []byte("v"))
	if lsn != 0 {
		t.Errorf("expected LSN 0, got %d", lsn)
	}
	if err := w.Commit(lsn); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	entries := w.Entries()
	if len(entries) != 1 || !entries[0].Committed {
		t.Error("expected 1 committed entry")
	}
}

func TestWAL_UncommittedIgnoredOnRecover(t *testing.T) {
	s := NewWALStore()
	s.Put("committed", []byte("yes"))

	// Simulate an uncommitted write: append without committing.
	s.WALLog().Append(WALPut, "uncommitted", []byte("no"))

	fresh := s.Recover()
	if _, ok := fresh.Get("committed"); !ok {
		t.Error("committed key should survive recovery")
	}
	if _, ok := fresh.Get("uncommitted"); ok {
		t.Error("uncommitted key should not survive recovery")
	}
}

func TestWALStore_PutGetDelete(t *testing.T) {
	s := NewWALStore()
	s.Put("foo", []byte("bar"))
	v, ok := s.Get("foo")
	if !ok || string(v) != "bar" {
		t.Errorf("expected bar, got %q ok=%v", v, ok)
	}
	s.Delete("foo")
	if _, ok := s.Get("foo"); ok {
		t.Error("key should be deleted")
	}
}

func TestWALStore_DeleteLogged(t *testing.T) {
	s := NewWALStore()
	s.Put("x", []byte("1"))
	s.Delete("x")
	entries := s.WALLog().Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	if entries[1].Op != WALDelete {
		t.Error("second entry should be WALDelete")
	}
}

func TestWALStore_RecoverDelete(t *testing.T) {
	s := NewWALStore()
	s.Put("a", []byte("1"))
	s.Put("b", []byte("2"))
	s.Delete("a")
	fresh := s.Recover()
	if _, ok := fresh.Get("a"); ok {
		t.Error("deleted key should not appear after recovery")
	}
	if v, ok := fresh.Get("b"); !ok || string(v) != "2" {
		t.Errorf("expected b=2, got %q ok=%v", v, ok)
	}
}

func TestWAL_Concurrent(t *testing.T) {
	s := NewWALStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := "key"
			s.Put(key, []byte{byte(i)})
		}()
	}
	wg.Wait()
	if s.WALLog().Len() < 50 {
		t.Errorf("expected at least 50 log entries, got %d", s.WALLog().Len())
	}
}

// =============================================================================
// FS-002: Hard Links & Reference Counting Tests
// =============================================================================

func TestHardLink_CreateRead(t *testing.T) {
	fs := NewHardLinkFS()
	_, err := fs.Create("a.txt", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := fs.Read("a.txt")
	if err != nil || string(data) != "hello" {
		t.Errorf("expected hello, got %q err=%v", data, err)
	}
}

func TestHardLink_LinkSharesInode(t *testing.T) {
	fs := NewHardLinkFS()
	fs.Create("original", []byte("data"))
	if err := fs.Link("original", "alias"); err != nil {
		t.Fatal(err)
	}
	idOrig, _ := fs.InodeFor("original")
	idAlias, _ := fs.InodeFor("alias")
	if idOrig != idAlias {
		t.Error("hard link should point to same inode")
	}
	rc, _ := fs.RefCountFor("original")
	if rc != 2 {
		t.Errorf("expected refcount 2, got %d", rc)
	}
}

func TestHardLink_UnlinkDecrements(t *testing.T) {
	fs := NewHardLinkFS()
	fs.Create("f", []byte("x"))
	fs.Link("f", "g")
	fs.Unlink("f")
	rc, _ := fs.RefCountFor("g")
	if rc != 1 {
		t.Errorf("expected refcount 1, got %d", rc)
	}
	data, err := fs.Read("g")
	if err != nil || string(data) != "x" {
		t.Errorf("data should survive while refcount > 0: %q %v", data, err)
	}
}

func TestHardLink_DataFreedWhenRefZero(t *testing.T) {
	fs := NewHardLinkFS()
	fs.Create("tmp", []byte("secret"))
	fs.Link("tmp", "tmp2")
	fs.Unlink("tmp")
	fs.Unlink("tmp2")
	// After both unlinks, the inode should be gone.
	if _, err := fs.Read("tmp"); err == nil {
		t.Error("expected error reading deleted file")
	}
	if _, err := fs.Read("tmp2"); err == nil {
		t.Error("expected error reading deleted file")
	}
}

func TestHardLink_DuplicateName(t *testing.T) {
	fs := NewHardLinkFS()
	fs.Create("dup", []byte("a"))
	if _, err := fs.Create("dup", []byte("b")); err == nil {
		t.Error("expected error creating duplicate name")
	}
}

func TestHardLink_Concurrent(t *testing.T) {
	fs := NewHardLinkFS()
	fs.Create("shared", []byte("data"))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			linkName := "link" + string(rune('A'+i))
			fs.Link("shared", linkName)
			fs.Read(linkName)
			fs.Unlink(linkName)
		}()
	}
	wg.Wait()
}

// =============================================================================
// FS-003: Buffer Cache Tests
// =============================================================================

func TestBufferCache_ReadMiss(t *testing.T) {
	bm := NewBlockManager(8, 64)
	copy(bm.Blocks[0].Data, []byte("hello"))
	bc := NewBufferCache(bm, 4)

	data, err := bc.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:5]) != "hello" {
		t.Errorf("expected hello, got %q", data[:5])
	}
	if bc.HitRate() != 0 {
		t.Errorf("expected 0 hit rate on first access, got %f", bc.HitRate())
	}
}

func TestBufferCache_ReadHit(t *testing.T) {
	bm := NewBlockManager(8, 64)
	bc := NewBufferCache(bm, 4)
	bc.Read(0)
	bc.Read(0)
	hr := bc.HitRate()
	if hr < 0.5 {
		t.Errorf("expected hit rate >= 0.5, got %f", hr)
	}
}

func TestBufferCache_Write(t *testing.T) {
	bm := NewBlockManager(8, 64)
	bc := NewBufferCache(bm, 4)

	if err := bc.Write(0, []byte("world")); err != nil {
		t.Fatal(err)
	}
	data, _ := bc.Read(0)
	if string(data[:5]) != "world" {
		t.Errorf("expected world, got %q", data[:5])
	}
	// Before flush, block manager should not have the data yet.
	// (depends on eviction, not guaranteed — just verify flush applies it)
	bc.Flush()
	if string(bm.Blocks[0].Data[:5]) != "world" {
		t.Errorf("after flush, BM should have world, got %q", bm.Blocks[0].Data[:5])
	}
}

func TestBufferCache_Eviction(t *testing.T) {
	bm := NewBlockManager(16, 64)
	bc := NewBufferCache(bm, 3) // capacity = 3

	for i := 0; i < 5; i++ {
		bc.Read(i)
	}
	if bc.Size() > 3 {
		t.Errorf("cache size should not exceed capacity; got %d", bc.Size())
	}
}

func TestBufferCache_DirtyEviction(t *testing.T) {
	bm := NewBlockManager(8, 64)
	bc := NewBufferCache(bm, 2)

	bc.Write(0, []byte("data0"))
	bc.Write(1, []byte("data1"))
	// Evict block 0 by adding block 2 (capacity=2, LRU=0).
	bc.Read(2)
	// Dirty write-back should have happened on eviction.
	if string(bm.Blocks[0].Data[:5]) != "data0" {
		t.Errorf("evicted dirty block should be written back, got %q", bm.Blocks[0].Data[:5])
	}
}

func TestBufferCache_Concurrent(t *testing.T) {
	bm := NewBlockManager(16, 64)
	bc := NewBufferCache(bm, 8)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			bn := i % 16
			bc.Write(bn, []byte("concurrent"))
			bc.Read(bn)
		}()
	}
	wg.Wait()
	bc.Flush()
}

func TestBufferCache_OutOfRange(t *testing.T) {
	bm := NewBlockManager(4, 64)
	bc := NewBufferCache(bm, 4)
	if _, err := bc.Read(100); err == nil {
		t.Error("expected error for out-of-range block")
	}
	if err := bc.Write(100, []byte("x")); err == nil {
		t.Error("expected error for out-of-range write")
	}
}

// =============================================================================
// FS-004: Copy-on-Write Tests
// =============================================================================

func TestCoW_AllocateRead(t *testing.T) {
	m := NewCoWManager()
	m.AllocatePage(1, 0, []byte("hello"))
	data, err := m.Read(1, 0)
	if err != nil || string(data) != "hello" {
		t.Errorf("expected hello, got %q err=%v", data, err)
	}
}

func TestCoW_ForkSharesPages(t *testing.T) {
	m := NewCoWManager()
	m.AllocatePage(1, 0, []byte("shared"))
	m.Fork(1, 2)
	data, err := m.Read(2, 0)
	if err != nil || string(data) != "shared" {
		t.Errorf("child should see parent's data: %q %v", data, err)
	}
}

func TestCoW_WriteTriggersPrivateCopy(t *testing.T) {
	m := NewCoWManager()
	m.AllocatePage(1, 0, []byte("shared"))
	m.Fork(1, 2)

	faultsBefore := m.WriteFaults
	// Child writes → should trigger CoW fault.
	m.Write(2, 0, []byte("child!"))
	if m.WriteFaults <= faultsBefore {
		t.Error("expected CoW write fault to be recorded")
	}
	// Child sees its private data.
	childData, _ := m.Read(2, 0)
	if string(childData) != "child!" {
		t.Errorf("child should see child!, got %q", childData)
	}
	// Parent still sees original.
	parentData, _ := m.Read(1, 0)
	if string(parentData) != "shared" {
		t.Errorf("parent should still see shared, got %q", parentData)
	}
}

func TestCoW_SoleOwnerNoFault(t *testing.T) {
	m := NewCoWManager()
	m.AllocatePage(1, 0, []byte("aaaaa"))
	faultsBefore := m.WriteFaults
	// Single owner write should not cause a CoW fault.
	m.Write(1, 0, []byte("bbbbb"))
	if m.WriteFaults != faultsBefore {
		t.Error("sole-owner write should not trigger CoW fault")
	}
	data, _ := m.Read(1, 0)
	if string(data) != "bbbbb" {
		t.Errorf("expected bbbbb, got %q", data)
	}
}

func TestCoW_Snapshot(t *testing.T) {
	m := NewCoWManager()
	m.AllocatePage(1, 0, []byte("page0"))
	m.AllocatePage(1, 1, []byte("page1"))

	snap := m.Snapshot(1)
	if len(snap) != 2 {
		t.Errorf("expected 2 pages in snapshot, got %d", len(snap))
	}
	if string(snap[0]) != "page0" {
		t.Errorf("expected page0, got %q", snap[0])
	}
}

func TestCoW_PageCount(t *testing.T) {
	m := NewCoWManager()
	m.AllocatePage(1, 0, []byte("a"))
	m.AllocatePage(1, 1, []byte("b"))
	if m.PageCount() != 2 {
		t.Errorf("expected 2 pages, got %d", m.PageCount())
	}
}

func TestCoW_Concurrent(t *testing.T) {
	m := NewCoWManager()
	for p := 0; p < 8; p++ {
		m.AllocatePage(0, p, []byte("base"))
	}
	var wg sync.WaitGroup
	for pid := 1; pid <= 8; pid++ {
		pid := pid
		m.Fork(0, pid)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pg := 0; pg < 8; pg++ {
				m.Write(pid, pg, []byte("mine"))
				m.Read(pid, pg)
			}
		}()
	}
	wg.Wait()
}
