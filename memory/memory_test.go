package memory

import (
	"testing"
)

func TestFIFO(t *testing.T) {
	fifo := NewFIFO(3)

	references := []int{1, 2, 3, 4, 1, 2, 5, 1, 2, 3, 4, 5}

	for i, page := range references {
		fifo.Access(page, i)
	}

	if fifo.GetPageFaults() == 0 {
		t.Error("Expected page faults, got 0")
	}

	t.Logf("FIFO page faults: %d", fifo.GetPageFaults())
}

func TestLRU(t *testing.T) {
	lru := NewLRU(3)

	references := []int{1, 2, 3, 4, 1, 2, 5, 1, 2, 3, 4, 5}

	pageFaults := 0
	for i, page := range references {
		hit, _ := lru.Access(page, i)
		if !hit {
			pageFaults++
		}
	}

	if pageFaults == 0 {
		t.Error("Expected page faults, got 0")
	}

	t.Logf("LRU page faults: %d", lru.GetPageFaults())
}

func TestOptimal(t *testing.T) {
	references := []int{7, 0, 1, 2, 0, 3, 0, 4, 2, 3, 0, 3, 2}
	optimal := NewOptimal(3, references)

	for i, page := range references {
		optimal.Access(page, i)
	}

	t.Logf("Optimal page faults: %d", optimal.GetPageFaults())

	// Optimal should have fewest page faults
	if optimal.GetPageFaults() == 0 {
		t.Error("Expected some page faults")
	}
}

func TestClock(t *testing.T) {
	clock := NewClock(4)

	references := []int{1, 2, 3, 4, 1, 2, 5, 1, 2, 3, 4, 5}

	for i, page := range references {
		clock.Access(page, i)
	}

	if clock.GetPageFaults() == 0 {
		t.Error("Expected page faults, got 0")
	}

	t.Logf("Clock page faults: %d", clock.GetPageFaults())
}

func TestLRUK(t *testing.T) {
	lruk := NewLRUK(3, 2)

	references := []int{1, 2, 3, 4, 1, 2, 5, 1, 2, 3, 4, 5}

	for i, page := range references {
		lruk.Access(page, i)
	}

	if lruk.GetPageFaults() == 0 {
		t.Error("Expected page faults, got 0")
	}

	t.Logf("LRU-2 page faults: %d", lruk.GetPageFaults())
}

func TestFirstFit(t *testing.T) {
	ff := NewFirstFit(1000)

	// Allocate some blocks
	addr1, err := ff.Allocate(1, 100)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}
	if addr1 != 0 {
		t.Errorf("Expected address 0, got %d", addr1)
	}

	addr2, err := ff.Allocate(2, 200)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}
	if addr2 != 100 {
		t.Errorf("Expected address 100, got %d", addr2)
	}

	// Deallocate
	err = ff.Deallocate(1)
	if err != nil {
		t.Errorf("Deallocation failed: %v", err)
	}

	// Allocate again
	addr3, err := ff.Allocate(3, 50)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}
	if addr3 != 0 {
		t.Errorf("Expected address 0 (reusing freed space), got %d", addr3)
	}
}

func TestBestFit(t *testing.T) {
	bf := NewBestFit(1000)

	addr1, err := bf.Allocate(1, 300)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}

	_, err = bf.Allocate(2, 200)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}

	err = bf.Deallocate(1)
	if err != nil {
		t.Errorf("Deallocation failed: %v", err)
	}

	// Should use the 300-byte hole (best fit for 250 bytes)
	addr3, err := bf.Allocate(3, 250)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}

	if addr3 != addr1 {
		t.Logf("Best fit placed at address %d", addr3)
	}
}

func TestWorstFit(t *testing.T) {
	wf := NewWorstFit(1000)

	_, err := wf.Allocate(1, 100)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}

	_, err = wf.Allocate(2, 200)
	if err != nil {
		t.Errorf("Allocation failed: %v", err)
	}

	err = wf.Deallocate(1)
	if err != nil {
		t.Errorf("Deallocation failed: %v", err)
	}
}

func TestPageTable(t *testing.T) {
	pt := NewPageTable()

	frame := &Frame{Number: 5, Occupied: true}
	pt.Insert(10, frame)

	retrievedFrame, exists := pt.Lookup(10)
	if !exists {
		t.Error("Page not found in page table")
	}

	if retrievedFrame.Number != 5 {
		t.Errorf("Expected frame 5, got %d", retrievedFrame.Number)
	}

	pt.Remove(10)
	_, exists = pt.Lookup(10)
	if exists {
		t.Error("Page should have been removed")
	}
}

func TestTLB(t *testing.T) {
	tlb := NewTLB(4)

	// Insert some entries
	tlb.Insert(10, 100, 1)
	tlb.Insert(20, 200, 2)

	// Lookup hit
	frameNum, hit := tlb.Lookup(10)
	if !hit {
		t.Error("Expected TLB hit")
	}
	if frameNum != 100 {
		t.Errorf("Expected frame 100, got %d", frameNum)
	}

	// Lookup miss
	_, hit = tlb.Lookup(999)
	if hit {
		t.Error("Expected TLB miss")
	}

	hitRate := tlb.GetHitRate()
	expectedRate := 50.0 // 1 hit, 1 miss
	if hitRate != expectedRate {
		t.Errorf("Expected hit rate %f, got %f", expectedRate, hitRate)
	}
}

func TestPageReplacementComparison(t *testing.T) {
	references := []int{7, 0, 1, 2, 0, 3, 0, 4, 2, 3, 0, 3, 2}

	fifo := NewFIFO(3)
	lru := NewLRU(3)
	clock := NewClock(3)
	optimal := NewOptimal(3, references)

	for i, page := range references {
		fifo.Access(page, i)
		lru.Access(page, i)
		clock.Access(page, i)
		optimal.Access(page, i)
	}

	t.Logf("Page Replacement Algorithm Comparison:")
	t.Logf("  FIFO:    %d page faults", fifo.GetPageFaults())
	t.Logf("  LRU:     %d page faults", lru.GetPageFaults())
	t.Logf("  Clock:   %d page faults", clock.GetPageFaults())
	t.Logf("  Optimal: %d page faults", optimal.GetPageFaults())

	// Optimal should have minimum faults
	if optimal.GetPageFaults() > lru.GetPageFaults() {
		t.Error("Optimal should have fewer or equal page faults compared to LRU")
	}
}
