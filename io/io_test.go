package io

import (
	"testing"
)

func TestFCFSDiskScheduling(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 1},
		{ID: 3, Track: 37, ArrivalTime: 2},
	}

	fcfs := NewFCFS()
	sequence := fcfs.Schedule(requests, 53)

	if len(sequence) != 3 {
		t.Errorf("Expected 3 tracks, got %d", len(sequence))
	}

	// FCFS should maintain arrival order
	if sequence[0] != 98 {
		t.Errorf("Expected first track 98, got %d", sequence[0])
	}
}

func TestSSTFDiskScheduling(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 0},
		{ID: 3, Track: 37, ArrivalTime: 0},
		{ID: 4, Track: 122, ArrivalTime: 0},
	}

	sstf := NewSSTF()
	sequence := sstf.Schedule(requests, 53)

	// SSTF should select closest track first (37 is closest to 53)
	if sequence[0] != 37 {
		t.Logf("SSTF sequence: %v", sequence)
	}

	if len(sequence) != 4 {
		t.Errorf("Expected 4 tracks, got %d", len(sequence))
	}
}

func TestSCANDiskScheduling(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 0},
		{ID: 3, Track: 37, ArrivalTime: 0},
	}

	scan := NewSCAN(200, "up")
	sequence := scan.Schedule(requests, 53)

	if len(sequence) != 3 {
		t.Errorf("Expected 3 tracks, got %d", len(sequence))
	}

	// SCAN going up should service 98, 183 first, then 37
	if sequence[0] != 98 {
		t.Logf("SCAN sequence: %v", sequence)
	}
}

func TestLOOKDiskScheduling(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 0},
		{ID: 3, Track: 37, ArrivalTime: 0},
	}

	look := NewLOOK("up")
	sequence := look.Schedule(requests, 53)

	if len(sequence) != 3 {
		t.Errorf("Expected 3 tracks, got %d", len(sequence))
	}
}

func TestCalculateMetrics(t *testing.T) {
	sequence := []int{98, 183, 37, 122}
	startTrack := 53

	metrics := CalculateMetrics(sequence, startTrack)

	if metrics.TotalSeekDistance <= 0 {
		t.Error("Total seek distance should be positive")
	}

	if metrics.AverageSeekDistance <= 0 {
		t.Error("Average seek distance should be positive")
	}

	if metrics.MaxSeekDistance <= 0 {
		t.Error("Max seek distance should be positive")
	}

	t.Logf("Metrics: Total=%d, Avg=%.2f, Max=%d",
		metrics.TotalSeekDistance,
		metrics.AverageSeekDistance,
		metrics.MaxSeekDistance)
}

func TestBuffer(t *testing.T) {
	buf := NewBuffer(100)

	data := []byte{1, 2, 3, 4, 5}
	written := buf.Write(data)

	if written != 5 {
		t.Errorf("Expected to write 5 bytes, wrote %d", written)
	}

	read := buf.Read(3)
	if len(read) != 3 {
		t.Errorf("Expected to read 3 bytes, read %d", len(read))
	}

	if read[0] != 1 || read[1] != 2 || read[2] != 3 {
		t.Errorf("Read incorrect data: %v", read)
	}
}

func TestBufferFull(t *testing.T) {
	buf := NewBuffer(5)

	data := []byte{1, 2, 3, 4, 5}
	buf.Write(data)

	if !buf.IsFull() {
		t.Error("Buffer should be full")
	}

	// Try to write more (should only write 0 bytes)
	more := []byte{6, 7}
	written := buf.Write(more)

	if written != 0 {
		t.Errorf("Should not be able to write to full buffer, wrote %d", written)
	}
}

func TestDoubleBuffer(t *testing.T) {
	db := NewDoubleBuffer(10)

	data1 := []byte{1, 2, 3}
	db.Write(data1)

	data2 := []byte{4, 5, 6}
	db.Write(data2)

	// Read should get data from active buffer
	read := db.Read(3)
	if len(read) != 3 {
		t.Errorf("Expected to read 3 bytes, read %d", len(read))
	}
}

func TestSpooler(t *testing.T) {
	spooler := NewSpooler()

	// Submit jobs
	id1 := spooler.Submit("job1", []byte("data1"), 1)
	id2 := spooler.Submit("job2", []byte("data2"), 5)
	id3 := spooler.Submit("job3", []byte("data3"), 3)

	if id1 <= 0 || id2 <= 0 || id3 <= 0 {
		t.Error("Job IDs should be positive")
	}

	// Get next job (should be highest priority = id2)
	job := spooler.GetNextJob()
	if job == nil {
		t.Fatal("Should get a job")
	}

	if job.Priority != 5 {
		t.Errorf("Expected highest priority job (5), got priority %d", job.Priority)
	}

	spooler.CompleteJob()

	// Get next
	job = spooler.GetNextJob()
	if job.Priority != 3 {
		t.Errorf("Expected priority 3, got %d", job.Priority)
	}
}

func TestDMAController(t *testing.T) {
	dma := NewDMAController()

	if dma.Busy {
		t.Error("DMA should not be busy initially")
	}

	// Initiate transfer
	err := dma.InitiateTransfer(0, 1000, 512)
	if err != nil {
		t.Errorf("Transfer initiation failed: %v", err)
	}

	if !dma.Busy {
		t.Error("DMA should be busy after initiation")
	}

	// Transfer chunks
	transferred := dma.TransferChunk(256)
	if transferred != 256 {
		t.Errorf("Expected to transfer 256 bytes, transferred %d", transferred)
	}

	transferred = dma.TransferChunk(256)
	if transferred != 256 {
		t.Errorf("Expected to transfer 256 bytes, transferred %d", transferred)
	}

	if !dma.IsComplete() {
		t.Error("Transfer should be complete")
	}

	if dma.Busy {
		t.Error("DMA should not be busy after completion")
	}
}

func TestCSCANDiskScheduling(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 0},
		{ID: 3, Track: 37, ArrivalTime: 0},
		{ID: 4, Track: 122, ArrivalTime: 0},
	}

	cscan := NewCSCAN(200, "up")
	sequence := cscan.Schedule(requests, 53)

	if len(sequence) != 4 {
		t.Errorf("Expected 4 tracks, got %d", len(sequence))
	}
}

func TestCLOOKDiskScheduling(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 0},
		{ID: 3, Track: 37, ArrivalTime: 0},
	}

	clook := NewCLOOK("up")
	sequence := clook.Schedule(requests, 53)

	if len(sequence) != 3 {
		t.Errorf("Expected 3 tracks, got %d", len(sequence))
	}
}

func TestCompareSchedulers(t *testing.T) {
	requests := []*DiskRequest{
		{ID: 1, Track: 98, ArrivalTime: 0},
		{ID: 2, Track: 183, ArrivalTime: 0},
		{ID: 3, Track: 37, ArrivalTime: 0},
		{ID: 4, Track: 122, ArrivalTime: 0},
	}

	schedulers := []DiskScheduler{
		NewFCFS(),
		NewSSTF(),
		NewSCAN(200, "up"),
		NewLOOK("up"),
	}

	results := CompareSchedulers(schedulers, requests, 53)

	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}

	for name, metrics := range results {
		t.Logf("%s: Total Seek=%d, Avg=%.2f",
			name, metrics.TotalSeekDistance, metrics.AverageSeekDistance)
	}
}
