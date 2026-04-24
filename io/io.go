/*
I/O and Disk Scheduling
========================

Implementation of disk scheduling algorithms for I/O optimization.

Applications:
- Hard disk drive optimization
- SSD request scheduling
- Database buffer management
- Storage system optimization
*/

package io

import (
	"math"
	"sort"
)

// =============================================================================
// Disk Request
// =============================================================================

// DiskRequest represents an I/O request
type DiskRequest struct {
	ID        int
	Track     int
	ArrivalTime int
	Completed bool
}

// =============================================================================
// Disk Scheduler Interface
// =============================================================================

// DiskScheduler interface for different scheduling algorithms
type DiskScheduler interface {
	Schedule(requests []*DiskRequest, currentTrack int) []int
	Name() string
}

// =============================================================================
// FCFS (First-Come, First-Served)
// =============================================================================

// FCFS implements First-Come, First-Served disk scheduling
type FCFS struct{}

// NewFCFS creates a new FCFS scheduler
func NewFCFS() *FCFS {
	return &FCFS{}
}

func (f *FCFS) Name() string {
	return "FCFS (First-Come, First-Served)"
}

func (f *FCFS) Schedule(requests []*DiskRequest, currentTrack int) []int {
	// Sort by arrival time
	sorted := make([]*DiskRequest, len(requests))
	copy(sorted, requests)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ArrivalTime < sorted[j].ArrivalTime
	})

	sequence := make([]int, len(sorted))
	for i, req := range sorted {
		sequence[i] = req.Track
	}

	return sequence
}

// =============================================================================
// SSTF (Shortest Seek Time First)
// =============================================================================

// SSTF implements Shortest Seek Time First scheduling
type SSTF struct{}

// NewSSTF creates a new SSTF scheduler
func NewSSTF() *SSTF {
	return &SSTF{}
}

func (s *SSTF) Name() string {
	return "SSTF (Shortest Seek Time First)"
}

func (s *SSTF) Schedule(requests []*DiskRequest, currentTrack int) []int {
	remaining := make([]*DiskRequest, len(requests))
	copy(remaining, requests)

	sequence := make([]int, 0, len(requests))
	current := currentTrack

	for len(remaining) > 0 {
		// Find request with shortest seek time from current position
		minDist := math.MaxInt32
		minIdx := -1

		for i, req := range remaining {
			dist := abs(req.Track - current)
			if dist < minDist {
				minDist = dist
				minIdx = i
			}
		}

		// Add to sequence
		sequence = append(sequence, remaining[minIdx].Track)
		current = remaining[minIdx].Track

		// Remove from remaining
		remaining = append(remaining[:minIdx], remaining[minIdx+1:]...)
	}

	return sequence
}

// =============================================================================
// SCAN (Elevator Algorithm)
// =============================================================================

// SCAN implements SCAN (elevator) disk scheduling
type SCAN struct {
	MaxTrack  int
	Direction string // "up" or "down"
}

// NewSCAN creates a new SCAN scheduler
func NewSCAN(maxTrack int, direction string) *SCAN {
	return &SCAN{
		MaxTrack:  maxTrack,
		Direction: direction,
	}
}

func (s *SCAN) Name() string {
	return "SCAN (Elevator)"
}

func (s *SCAN) Schedule(requests []*DiskRequest, currentTrack int) []int {
	// Separate requests into two groups
	var lower, higher []*DiskRequest

	for _, req := range requests {
		if req.Track < currentTrack {
			lower = append(lower, req)
		} else {
			higher = append(higher, req)
		}
	}

	// Sort both groups
	sort.Slice(lower, func(i, j int) bool {
		return lower[i].Track > lower[j].Track // Descending
	})
	sort.Slice(higher, func(i, j int) bool {
		return higher[i].Track < higher[j].Track // Ascending
	})

	sequence := make([]int, 0, len(requests))

	if s.Direction == "up" {
		// Service higher tracks first, then lower
		for _, req := range higher {
			sequence = append(sequence, req.Track)
		}
		// Go to end, then reverse
		for _, req := range lower {
			sequence = append(sequence, req.Track)
		}
	} else {
		// Service lower tracks first, then higher
		for _, req := range lower {
			sequence = append(sequence, req.Track)
		}
		// Go to beginning, then reverse
		for _, req := range higher {
			sequence = append(sequence, req.Track)
		}
	}

	return sequence
}

// =============================================================================
// C-SCAN (Circular SCAN)
// =============================================================================

// CSCAN implements Circular SCAN disk scheduling
type CSCAN struct {
	MaxTrack  int
	Direction string
}

// NewCSCAN creates a new C-SCAN scheduler
func NewCSCAN(maxTrack int, direction string) *CSCAN {
	return &CSCAN{
		MaxTrack:  maxTrack,
		Direction: direction,
	}
}

func (c *CSCAN) Name() string {
	return "C-SCAN (Circular SCAN)"
}

func (c *CSCAN) Schedule(requests []*DiskRequest, currentTrack int) []int {
	// Separate requests
	var lower, higher []*DiskRequest

	for _, req := range requests {
		if req.Track < currentTrack {
			lower = append(lower, req)
		} else {
			higher = append(higher, req)
		}
	}

	// Sort both groups in ascending order
	sort.Slice(lower, func(i, j int) bool {
		return lower[i].Track < lower[j].Track
	})
	sort.Slice(higher, func(i, j int) bool {
		return higher[i].Track < higher[j].Track
	})

	sequence := make([]int, 0, len(requests))

	if c.Direction == "up" {
		// Service higher tracks, then jump to beginning and service lower
		for _, req := range higher {
			sequence = append(sequence, req.Track)
		}
		// Jump to beginning (0), then service lower tracks
		for _, req := range lower {
			sequence = append(sequence, req.Track)
		}
	} else {
		// Service lower tracks (descending), then jump to end and service higher
		for i := len(lower) - 1; i >= 0; i-- {
			sequence = append(sequence, lower[i].Track)
		}
		// Jump to end, then service higher tracks
		for i := len(higher) - 1; i >= 0; i-- {
			sequence = append(sequence, higher[i].Track)
		}
	}

	return sequence
}

// =============================================================================
// LOOK
// =============================================================================

// LOOK implements LOOK disk scheduling (SCAN without going to end)
type LOOK struct {
	Direction string
}

// NewLOOK creates a new LOOK scheduler
func NewLOOK(direction string) *LOOK {
	return &LOOK{Direction: direction}
}

func (l *LOOK) Name() string {
	return "LOOK"
}

func (l *LOOK) Schedule(requests []*DiskRequest, currentTrack int) []int {
	var lower, higher []*DiskRequest

	for _, req := range requests {
		if req.Track < currentTrack {
			lower = append(lower, req)
		} else {
			higher = append(higher, req)
		}
	}

	sort.Slice(lower, func(i, j int) bool {
		return lower[i].Track > lower[j].Track
	})
	sort.Slice(higher, func(i, j int) bool {
		return higher[i].Track < higher[j].Track
	})

	sequence := make([]int, 0, len(requests))

	if l.Direction == "up" {
		for _, req := range higher {
			sequence = append(sequence, req.Track)
		}
		for _, req := range lower {
			sequence = append(sequence, req.Track)
		}
	} else {
		for _, req := range lower {
			sequence = append(sequence, req.Track)
		}
		for _, req := range higher {
			sequence = append(sequence, req.Track)
		}
	}

	return sequence
}

// =============================================================================
// C-LOOK (Circular LOOK)
// =============================================================================

// CLOOK implements Circular LOOK disk scheduling
type CLOOK struct {
	Direction string
}

// NewCLOOK creates a new C-LOOK scheduler
func NewCLOOK(direction string) *CLOOK {
	return &CLOOK{Direction: direction}
}

func (c *CLOOK) Name() string {
	return "C-LOOK (Circular LOOK)"
}

func (c *CLOOK) Schedule(requests []*DiskRequest, currentTrack int) []int {
	var lower, higher []*DiskRequest

	for _, req := range requests {
		if req.Track < currentTrack {
			lower = append(lower, req)
		} else {
			higher = append(higher, req)
		}
	}

	sort.Slice(lower, func(i, j int) bool {
		return lower[i].Track < lower[j].Track
	})
	sort.Slice(higher, func(i, j int) bool {
		return higher[i].Track < higher[j].Track
	})

	sequence := make([]int, 0, len(requests))

	if c.Direction == "up" {
		for _, req := range higher {
			sequence = append(sequence, req.Track)
		}
		for _, req := range lower {
			sequence = append(sequence, req.Track)
		}
	} else {
		for i := len(lower) - 1; i >= 0; i-- {
			sequence = append(sequence, lower[i].Track)
		}
		for i := len(higher) - 1; i >= 0; i-- {
			sequence = append(sequence, higher[i].Track)
		}
	}

	return sequence
}

// =============================================================================
// Performance Metrics
// =============================================================================

// SchedulerMetrics holds performance metrics for disk scheduling
type SchedulerMetrics struct {
	TotalSeekDistance int
	AverageSeekDistance float64
	MaxSeekDistance   int
	Variance          float64
}

// CalculateMetrics computes performance metrics for a scheduling sequence
func CalculateMetrics(sequence []int, startTrack int) SchedulerMetrics {
	if len(sequence) == 0 {
		return SchedulerMetrics{}
	}

	totalSeek := 0
	maxSeek := 0
	seeks := make([]int, len(sequence))

	current := startTrack
	for i, track := range sequence {
		seek := abs(track - current)
		seeks[i] = seek
		totalSeek += seek
		if seek > maxSeek {
			maxSeek = seek
		}
		current = track
	}

	avgSeek := float64(totalSeek) / float64(len(sequence))

	// Calculate variance
	variance := 0.0
	for _, seek := range seeks {
		diff := float64(seek) - avgSeek
		variance += diff * diff
	}
	variance /= float64(len(sequence))

	return SchedulerMetrics{
		TotalSeekDistance:   totalSeek,
		AverageSeekDistance: avgSeek,
		MaxSeekDistance:     maxSeek,
		Variance:            variance,
	}
}

// =============================================================================
// I/O Buffering
// =============================================================================

// Buffer represents an I/O buffer
type Buffer struct {
	Data     []byte
	Size     int
	Used     int
	Position int
}

// NewBuffer creates a new I/O buffer
func NewBuffer(size int) *Buffer {
	return &Buffer{
		Data:     make([]byte, size),
		Size:     size,
		Used:     0,
		Position: 0,
	}
}

// Write writes data to buffer
func (b *Buffer) Write(data []byte) int {
	available := b.Size - b.Used
	toWrite := len(data)
	if toWrite > available {
		toWrite = available
	}

	copy(b.Data[b.Used:], data[:toWrite])
	b.Used += toWrite

	return toWrite
}

// Read reads data from buffer
func (b *Buffer) Read(size int) []byte {
	available := b.Used - b.Position
	toRead := size
	if toRead > available {
		toRead = available
	}

	data := make([]byte, toRead)
	copy(data, b.Data[b.Position:b.Position+toRead])
	b.Position += toRead

	return data
}

// Flush clears the buffer
func (b *Buffer) Flush() {
	b.Used = 0
	b.Position = 0
}

// IsFull returns whether buffer is full
func (b *Buffer) IsFull() bool {
	return b.Used >= b.Size
}

// IsEmpty returns whether buffer is empty
func (b *Buffer) IsEmpty() bool {
	return b.Position >= b.Used
}

// =============================================================================
// Double Buffering
// =============================================================================

// DoubleBuffer implements double buffering for I/O
type DoubleBuffer struct {
	Buffer1  *Buffer
	Buffer2  *Buffer
	Active   *Buffer
	Inactive *Buffer
}

// NewDoubleBuffer creates a double buffer
func NewDoubleBuffer(bufferSize int) *DoubleBuffer {
	buffer1 := NewBuffer(bufferSize)
	buffer2 := NewBuffer(bufferSize)

	return &DoubleBuffer{
		Buffer1:  buffer1,
		Buffer2:  buffer2,
		Active:   buffer1,
		Inactive: buffer2,
	}
}

// Swap swaps active and inactive buffers
func (db *DoubleBuffer) Swap() {
	db.Active, db.Inactive = db.Inactive, db.Active
	db.Inactive.Flush()
}

// Write writes to active buffer
func (db *DoubleBuffer) Write(data []byte) int {
	written := db.Active.Write(data)
	if db.Active.IsFull() {
		db.Swap()
	}
	return written
}

// Read reads from active buffer
func (db *DoubleBuffer) Read(size int) []byte {
	data := db.Active.Read(size)
	if db.Active.IsEmpty() {
		db.Swap()
	}
	return data
}

// =============================================================================
// Spooling (Simultaneous Peripheral Operations OnLine)
// =============================================================================

// SpoolEntry represents a spooled job
type SpoolEntry struct {
	ID       int
	JobName  string
	Data     []byte
	Priority int
	Status   string // "waiting", "processing", "completed"
}

// Spooler manages print/I/O spooling
type Spooler struct {
	Queue      []*SpoolEntry
	NextID     int
	Processing *SpoolEntry
}

// NewSpooler creates a new spooler
func NewSpooler() *Spooler {
	return &Spooler{
		Queue:  make([]*SpoolEntry, 0),
		NextID: 1,
	}
}

// Submit submits a job to spool
func (s *Spooler) Submit(jobName string, data []byte, priority int) int {
	entry := &SpoolEntry{
		ID:       s.NextID,
		JobName:  jobName,
		Data:     data,
		Priority: priority,
		Status:   "waiting",
	}

	s.Queue = append(s.Queue, entry)
	s.NextID++

	// Sort by priority (higher priority first)
	sort.Slice(s.Queue, func(i, j int) bool {
		return s.Queue[i].Priority > s.Queue[j].Priority
	})

	return entry.ID
}

// GetNextJob gets the next job to process
func (s *Spooler) GetNextJob() *SpoolEntry {
	if len(s.Queue) == 0 {
		return nil
	}

	job := s.Queue[0]
	s.Queue = s.Queue[1:]
	job.Status = "processing"
	s.Processing = job

	return job
}

// CompleteJob marks current job as completed
func (s *Spooler) CompleteJob() {
	if s.Processing != nil {
		s.Processing.Status = "completed"
		s.Processing = nil
	}
}

// =============================================================================
// DMA (Direct Memory Access) Simulation
// =============================================================================

// DMAController simulates a DMA controller
type DMAController struct {
	SourceAddr      int
	DestAddr        int
	TransferSize    int
	BytesTransferred int
	Busy            bool
}

// NewDMAController creates a new DMA controller
func NewDMAController() *DMAController {
	return &DMAController{
		Busy: false,
	}
}

// InitiateTransfer starts a DMA transfer
func (dma *DMAController) InitiateTransfer(source, dest, size int) error {
	if dma.Busy {
		return nil
	}

	dma.SourceAddr = source
	dma.DestAddr = dest
	dma.TransferSize = size
	dma.BytesTransferred = 0
	dma.Busy = true

	return nil
}

// TransferChunk transfers a chunk of data
func (dma *DMAController) TransferChunk(chunkSize int) int {
	if !dma.Busy {
		return 0
	}

	remaining := dma.TransferSize - dma.BytesTransferred
	toTransfer := chunkSize
	if toTransfer > remaining {
		toTransfer = remaining
	}

	dma.BytesTransferred += toTransfer

	if dma.BytesTransferred >= dma.TransferSize {
		dma.Busy = false
	}

	return toTransfer
}

// IsComplete returns whether transfer is complete
func (dma *DMAController) IsComplete() bool {
	return !dma.Busy && dma.BytesTransferred >= dma.TransferSize
}

// =============================================================================
// Helper Functions
// =============================================================================

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// CompareSchedulers compares multiple disk schedulers
func CompareSchedulers(schedulers []DiskScheduler, requests []*DiskRequest, startTrack int) map[string]SchedulerMetrics {
	results := make(map[string]SchedulerMetrics)

	for _, scheduler := range schedulers {
		sequence := scheduler.Schedule(requests, startTrack)
		metrics := CalculateMetrics(sequence, startTrack)
		results[scheduler.Name()] = metrics
	}

	return results
}
