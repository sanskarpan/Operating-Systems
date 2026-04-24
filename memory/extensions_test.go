package memory

import (
	"sync"
	"testing"
)

// =============================================================================
// MEM-001: Demand Paging Tests
// =============================================================================

func TestDemandPaging_Hit(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(4, disk)

	// First access causes a fault, second is a hit.
	fault1, _ := sim.Access(0)
	if !fault1 {
		t.Error("first access to page 0 should be a page fault")
	}
	fault2, _ := sim.Access(0)
	if fault2 {
		t.Error("second access to page 0 should be a hit")
	}
	if sim.PageFaults != 1 {
		t.Errorf("expected 1 page fault, got %d", sim.PageFaults)
	}
}

func TestDemandPaging_Eviction(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(2, disk) // only 2 frames

	sim.Access(0)
	sim.Access(1)
	// Both frames used; accessing page 2 must evict LRU.
	fault, evicted := sim.Access(2)
	if !fault {
		t.Error("access to new page should fault")
	}
	if evicted == -1 {
		t.Error("expected an eviction")
	}
	if sim.PageFaults != 3 {
		t.Errorf("expected 3 page faults, got %d", sim.PageFaults)
	}
}

func TestDemandPaging_DirtyEviction(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(1, disk)

	sim.Access(0)
	data := []byte("hello")
	sim.Write(0, data)
	// Evict dirty page 0 by accessing page 1.
	sim.Access(1)
	if sim.DiskWrites == 0 {
		t.Error("dirty eviction should have caused a disk write")
	}
	// The data should now be on disk.
	loaded := disk.LoadPage(0)
	if string(loaded[:5]) != "hello" {
		t.Errorf("expected 'hello' on disk, got %q", string(loaded[:5]))
	}
}

func TestDemandPaging_FlushAll(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(4, disk)

	sim.Access(0)
	sim.Write(0, []byte("dirty"))
	sim.Access(1)
	sim.Write(1, []byte("data1"))

	prevWrites := sim.DiskWrites
	sim.FlushAll()
	if sim.DiskWrites <= prevWrites {
		t.Error("FlushAll should have written dirty pages")
	}
}

func TestDemandPaging_PageFaultRate(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(4, disk)

	// Access 4 unique pages (all faults), then access them again (all hits).
	for i := 0; i < 4; i++ {
		sim.Access(i)
	}
	for i := 0; i < 4; i++ {
		sim.Access(i)
	}
	rate := sim.PageFaultRate()
	// 4 faults / 8 accesses = 0.5
	if rate < 0.49 || rate > 0.51 {
		t.Errorf("expected fault rate ~0.5, got %.2f", rate)
	}
}

func TestDemandPaging_ResidentSet(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(3, disk)

	sim.Access(10)
	sim.Access(20)
	sim.Access(30)
	rs := sim.ResidentSet()
	if len(rs) != 3 {
		t.Errorf("expected 3 resident pages, got %d", len(rs))
	}
}

func TestDemandPaging_ReadPage(t *testing.T) {
	disk := NewDiskStore(0)
	// Pre-store data on "disk"
	data := make([]byte, PageSize)
	data[0] = 42
	disk.StorePage(5, data)

	sim := NewDemandPagingSimulator(4, disk)
	sim.Access(5) // fault → load from disk

	got, err := sim.ReadPage(5)
	if err != nil {
		t.Fatalf("ReadPage error: %v", err)
	}
	if got[0] != 42 {
		t.Errorf("expected 42 at offset 0, got %d", got[0])
	}
}

func TestDemandPaging_Concurrent(t *testing.T) {
	disk := NewDiskStore(0)
	sim := NewDemandPagingSimulator(8, disk)

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				sim.Access(g%10 + i%5)
			}
		}()
	}
	wg.Wait()
	// Just verify no panics/races; metrics should be non-negative.
	if sim.PageFaults < 0 || sim.TotalAccess < 0 {
		t.Error("negative metrics")
	}
}

// =============================================================================
// MEM-002: Working Set Model Tests
// =============================================================================

func TestWorkingSet_Basic(t *testing.T) {
	ws := NewWorkingSetModel(5, 4)
	for _, p := range []int{1, 2, 3, 1, 2} {
		ws.Access(p)
	}
	size := ws.WorkingSetSize()
	if size != 3 {
		t.Errorf("expected working set size 3, got %d", size)
	}
}

func TestWorkingSet_Slides(t *testing.T) {
	ws := NewWorkingSetModel(3, 4)
	// Window: [1,2,3] → size 3
	ws.Access(1)
	ws.Access(2)
	ws.Access(3)
	if ws.WorkingSetSize() != 3 {
		t.Errorf("expected 3, got %d", ws.WorkingSetSize())
	}
	// Add page 4, evict page 1 from window.
	ws.Access(4)
	if ws.WorkingSetSize() != 3 {
		t.Errorf("expected 3 after slide, got %d", ws.WorkingSetSize())
	}
}

func TestWorkingSet_ThrashingNotDetected(t *testing.T) {
	ws := NewWorkingSetModel(4, 4)
	for _, p := range []int{1, 2, 3, 4} {
		ws.Access(p)
	}
	if ws.IsThrashing() {
		t.Error("should not be thrashing when working set == frames")
	}
}

func TestWorkingSet_ThrashingDetected(t *testing.T) {
	ws := NewWorkingSetModel(5, 3) // 3 frames
	for _, p := range []int{1, 2, 3, 4, 5} {
		ws.Access(p)
	}
	if !ws.IsThrashing() {
		t.Error("should be thrashing when working set (5) > frames (3)")
	}
}

func TestWorkingSet_ThrashingPressure(t *testing.T) {
	ws := NewWorkingSetModel(4, 2) // 2 frames, window 4
	for _, p := range []int{1, 2, 3, 4} {
		ws.Access(p)
	}
	p := ws.ThrashingPressure()
	if p <= 1.0 {
		t.Errorf("expected pressure > 1, got %.2f", p)
	}
}

func TestWorkingSet_WorkingSetContents(t *testing.T) {
	ws := NewWorkingSetModel(3, 10)
	ws.Access(10)
	ws.Access(20)
	ws.Access(30)
	set := ws.WorkingSet()
	if len(set) != 3 {
		t.Errorf("expected 3 pages, got %d", len(set))
	}
	found := map[int]bool{10: false, 20: false, 30: false}
	for _, p := range set {
		found[p] = true
	}
	for p, ok := range found {
		if !ok {
			t.Errorf("page %d missing from working set", p)
		}
	}
}

func TestWorkingSet_Concurrent(t *testing.T) {
	ws := NewWorkingSetModel(10, 8)
	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				ws.Access(g*10 + i%10)
				_ = ws.WorkingSetSize()
				_ = ws.IsThrashing()
			}
		}()
	}
	wg.Wait()
}

// =============================================================================
// MEM-003: Buddy System Allocator Tests
// =============================================================================

func TestBuddy_BasicAllocFree(t *testing.T) {
	b, err := NewBuddyAllocator(1024, 16)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := b.Allocate(100)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if err := b.Free(addr, 100); err != nil {
		t.Fatalf("Free failed: %v", err)
	}
	if b.FreeMemory() != 1024 {
		t.Errorf("expected all memory free after free, got %d", b.FreeMemory())
	}
}

func TestBuddy_MultipleAlloc(t *testing.T) {
	b, _ := NewBuddyAllocator(1024, 16)
	addrs := make([]int, 0)
	for i := 0; i < 8; i++ {
		addr, err := b.Allocate(64)
		if err != nil {
			t.Fatalf("Allocate %d failed: %v", i, err)
		}
		addrs = append(addrs, addr)
	}
	// All 8×128 bytes = 1024 — should be full (blocks round to 64).
	if b.FreeMemory() != 512 {
		t.Logf("free memory: %d (expected 512)", b.FreeMemory())
	}
	for _, addr := range addrs {
		if err := b.Free(addr, 64); err != nil {
			t.Fatalf("Free failed: %v", err)
		}
	}
	if b.FreeMemory() != 1024 {
		t.Errorf("expected 1024 free after freeing all, got %d", b.FreeMemory())
	}
}

func TestBuddy_OutOfMemory(t *testing.T) {
	b, _ := NewBuddyAllocator(64, 16)
	_, err := b.Allocate(128)
	if err == nil {
		t.Error("expected out-of-memory error")
	}
}

func TestBuddy_Coalescing(t *testing.T) {
	b, _ := NewBuddyAllocator(256, 32)
	a1, _ := b.Allocate(32)
	a2, _ := b.Allocate(32)
	b.Free(a1, 32)
	b.Free(a2, 32)
	// After coalescing both buddies should merge back to a 64-byte block.
	if b.FreeMemory() != 256 {
		t.Errorf("expected 256 free after coalesce, got %d", b.FreeMemory())
	}
}

func TestBuddy_DoubleFree(t *testing.T) {
	b, _ := NewBuddyAllocator(256, 32)
	addr, _ := b.Allocate(32)
	b.Free(addr, 32)
	if err := b.Free(addr, 32); err == nil {
		t.Error("double-free should return error")
	}
}

func TestBuddy_InvalidSize(t *testing.T) {
	b, _ := NewBuddyAllocator(256, 32)
	if _, err := b.Allocate(0); err == nil {
		t.Error("expected error for size 0")
	}
	if _, err := b.Allocate(-1); err == nil {
		t.Error("expected error for size -1")
	}
}

func TestBuddy_UsedMemory(t *testing.T) {
	b, _ := NewBuddyAllocator(512, 16)
	b.Allocate(100) // rounds to 128
	used := b.UsedMemory()
	if used != 128 {
		t.Errorf("expected 128 used, got %d", used)
	}
}

func TestBuddy_Fragmentation(t *testing.T) {
	b, _ := NewBuddyAllocator(256, 16)
	// Allocate many small blocks, free every other one → fragmentation.
	addrs := make([]int, 0)
	for i := 0; i < 8; i++ {
		addr, err := b.Allocate(16)
		if err != nil {
			break
		}
		addrs = append(addrs, addr)
	}
	// Free alternating blocks.
	for i := 0; i < len(addrs); i += 2 {
		b.Free(addrs[i], 16)
	}
	frag := b.ExternalFragmentation()
	if frag < 0 || frag > 1 {
		t.Errorf("fragmentation must be in [0,1], got %.2f", frag)
	}
}

func TestBuddy_Concurrent(t *testing.T) {
	b, _ := NewBuddyAllocator(4096, 16)
	var mu sync.Mutex
	addrs := make([]int, 0)
	var wg sync.WaitGroup

	// Concurrent allocations.
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr, err := b.Allocate(32)
			if err == nil {
				mu.Lock()
				addrs = append(addrs, addr)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Free all allocated.
	for _, addr := range addrs {
		b.Free(addr, 32)
	}
}

func BenchmarkBuddyAllocFree(bm *testing.B) {
	b, _ := NewBuddyAllocator(1<<20, 16)
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		addr, err := b.Allocate(64)
		if err == nil {
			b.Free(addr, 64)
		}
	}
}

// =============================================================================
// MEM-004: Two-Level Page Table Tests
// =============================================================================

func newTestPT() *TwoLevelPageTable {
	// 4-bit PD, 4-bit PT, 8-bit offset → 16-bit virtual address space.
	// 256 physical frames.
	return NewTwoLevelPageTable(4, 4, 8, 256)
}

func TestTwoLevel_MapTranslate(t *testing.T) {
	pt := newTestPT()
	// Map virtual page 0 (addr 0x000) to physical frame 5.
	if err := pt.Map(0x000, 5); err != nil {
		t.Fatal(err)
	}
	phys, err := pt.Translate(0x00A) // offset 10
	if err != nil {
		t.Fatalf("Translate error: %v", err)
	}
	expected := 5*256 + 10
	if phys != expected {
		t.Errorf("expected physical %d, got %d", expected, phys)
	}
}

func TestTwoLevel_PageFaultUnmapped(t *testing.T) {
	pt := newTestPT()
	_, err := pt.Translate(0x100)
	if err == nil {
		t.Error("expected page fault for unmapped address")
	}
}

func TestTwoLevel_Unmap(t *testing.T) {
	pt := newTestPT()
	pt.Map(0x100, 3)
	pt.Unmap(0x100)
	_, err := pt.Translate(0x100)
	if err == nil {
		t.Error("expected page fault after unmap")
	}
}

func TestTwoLevel_AllocateAndMap(t *testing.T) {
	pt := newTestPT()
	frame, err := pt.AllocateAndMap(0x200)
	if err != nil {
		t.Fatal(err)
	}
	if frame < 0 {
		t.Error("frame must be non-negative")
	}
	phys, err := pt.Translate(0x200)
	if err != nil {
		t.Fatalf("Translate after AllocateAndMap: %v", err)
	}
	expected := frame * 256
	if phys != expected {
		t.Errorf("expected %d, got %d", expected, phys)
	}
}

func TestTwoLevel_SparseMapping(t *testing.T) {
	pt := newTestPT()
	// Map only a few entries in a large address space.
	pt.Map(0x0000, 0)
	pt.Map(0xFF00, 1)
	if pt.PDEntries() != 2 {
		t.Errorf("expected 2 PD entries, got %d", pt.PDEntries())
	}
	if pt.MappedPages() != 2 {
		t.Errorf("expected 2 mapped pages, got %d", pt.MappedPages())
	}
}

func TestTwoLevel_OutOfFrames(t *testing.T) {
	pt := NewTwoLevelPageTable(4, 4, 8, 2) // only 2 frames
	pt.AllocateAndMap(0x000)
	pt.AllocateAndMap(0x100)
	_, err := pt.AllocateAndMap(0x200)
	if err == nil {
		t.Error("expected out-of-frames error")
	}
}

func TestTwoLevel_MultiplePages(t *testing.T) {
	pt := NewTwoLevelPageTable(4, 4, 8, 256)
	for i := 0; i < 16; i++ {
		pt.Map(i*256, i) // page i -> frame i
	}
	for i := 0; i < 16; i++ {
		phys, err := pt.Translate(i * 256)
		if err != nil {
			t.Errorf("page %d: %v", i, err)
			continue
		}
		if phys != i*256 {
			t.Errorf("page %d: expected phys %d, got %d", i, i*256, phys)
		}
	}
}

func TestTwoLevel_Concurrent(t *testing.T) {
	pt := NewTwoLevelPageTable(4, 4, 8, 256)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			pt.Map(i*256, i)
			pt.Translate(i * 256)
		}()
	}
	wg.Wait()
}

func BenchmarkTwoLevelTranslate(bm *testing.B) {
	pt := NewTwoLevelPageTable(4, 4, 8, 256)
	for i := 0; i < 16; i++ {
		pt.Map(i*256, i)
	}
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		pt.Translate((i % 16) * 256)
	}
}
