/*
Classic Concurrency Problems
==============================

Implementation of classic concurrency problems and their solutions.

Applications:
- Understanding synchronization challenges
- Learning concurrency patterns
- Building thread-safe systems
- Resource management
*/

package concurrency

import (
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Producer-Consumer Problem
// =============================================================================

// ProducerConsumer implements the producer-consumer problem
type ProducerConsumer struct {
	buffer   []int
	capacity int
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	produced int
	consumed int
}

// NewProducerConsumer creates a new producer-consumer instance
func NewProducerConsumer(capacity int) *ProducerConsumer {
	pc := &ProducerConsumer{
		buffer:   make([]int, 0, capacity),
		capacity: capacity,
	}
	pc.notEmpty = sync.NewCond(&pc.mu)
	pc.notFull = sync.NewCond(&pc.mu)
	return pc
}

// Produce produces an item and adds it to the buffer
func (pc *ProducerConsumer) Produce(item int, producerID int) {
	pc.mu.Lock()

	for len(pc.buffer) >= pc.capacity {
		pc.notFull.Wait()
	}

	pc.buffer = append(pc.buffer, item)
	pc.produced++
	bufLen := len(pc.buffer)
	pc.notEmpty.Signal()
	pc.mu.Unlock()

	// Print after releasing the lock to avoid holding it during I/O.
	fmt.Printf("Producer %d: Produced item %d (buffer size: %d)\n", producerID, item, bufLen)
}

// Consume removes and returns an item from the buffer
func (pc *ProducerConsumer) Consume(consumerID int) int {
	pc.mu.Lock()

	for len(pc.buffer) == 0 {
		pc.notEmpty.Wait()
	}

	item := pc.buffer[0]
	pc.buffer = pc.buffer[1:]
	pc.consumed++
	bufLen := len(pc.buffer)
	pc.notFull.Signal()
	pc.mu.Unlock()

	// Print after releasing the lock to avoid holding it during I/O.
	fmt.Printf("Consumer %d: Consumed item %d (buffer size: %d)\n", consumerID, item, bufLen)

	return item
}

// RunProducers runs multiple producer goroutines
func (pc *ProducerConsumer) RunProducers(numProducers, itemsPerProducer int) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(numProducers)

	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				item := id*1000 + j
				pc.Produce(item, id)
				time.Sleep(time.Millisecond * 100)
			}
		}(i)
	}

	return &wg
}

// RunConsumers runs multiple consumer goroutines
func (pc *ProducerConsumer) RunConsumers(numConsumers, itemsPerConsumer int) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(numConsumers)

	for i := 0; i < numConsumers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itemsPerConsumer; j++ {
				pc.Consume(id)
				time.Sleep(time.Millisecond * 150)
			}
		}(i)
	}

	return &wg
}

// GetStats returns production/consumption statistics
func (pc *ProducerConsumer) GetStats() (produced, consumed int) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.produced, pc.consumed
}

// =============================================================================
// Dining Philosophers Problem
// =============================================================================

// Philosopher represents a philosopher in the dining philosophers problem
type Philosopher struct {
	ID         int
	LeftFork   *sync.Mutex
	RightFork  *sync.Mutex
	MealsEaten int
	mu         sync.Mutex
}

// DiningPhilosophers manages the dining philosophers problem
type DiningPhilosophers struct {
	Philosophers []*Philosopher
	Forks        []*sync.Mutex
	MaxEaters    chan struct{} // Limit concurrent eaters
}

// NewDiningPhilosophers creates a new dining philosophers instance
func NewDiningPhilosophers(numPhilosophers int) *DiningPhilosophers {
	forks := make([]*sync.Mutex, numPhilosophers)
	for i := 0; i < numPhilosophers; i++ {
		forks[i] = &sync.Mutex{}
	}

	philosophers := make([]*Philosopher, numPhilosophers)
	for i := 0; i < numPhilosophers; i++ {
		philosophers[i] = &Philosopher{
			ID:        i,
			LeftFork:  forks[i],
			RightFork: forks[(i+1)%numPhilosophers],
		}
	}

	return &DiningPhilosophers{
		Philosophers: philosophers,
		Forks:        forks,
		MaxEaters:    make(chan struct{}, numPhilosophers-1), // Allow N-1 to prevent deadlock
	}
}

// Think simulates thinking
func (p *Philosopher) Think() {
	fmt.Printf("Philosopher %d is thinking\n", p.ID)
	time.Sleep(time.Millisecond * time.Duration(50+p.ID*10))
}

// Eat simulates eating (with deadlock prevention)
func (p *Philosopher) Eat(maxEaters chan struct{}) {
	// Acquire permission to eat (prevents all from grabbing forks simultaneously)
	maxEaters <- struct{}{}
	defer func() { <-maxEaters }()

	// Resource ordering to prevent deadlock: always acquire lower-numbered fork first
	first, second := p.LeftFork, p.RightFork
	if p.ID%2 == 1 {
		first, second = second, first
	}

	first.Lock()
	fmt.Printf("Philosopher %d picked up first fork\n", p.ID)

	second.Lock()
	fmt.Printf("Philosopher %d picked up second fork\n", p.ID)

	fmt.Printf("Philosopher %d is eating\n", p.ID)
	time.Sleep(time.Millisecond * time.Duration(100+p.ID*10))

	p.mu.Lock()
	p.MealsEaten++
	p.mu.Unlock()

	second.Unlock()
	fmt.Printf("Philosopher %d put down second fork\n", p.ID)

	first.Unlock()
	fmt.Printf("Philosopher %d put down first fork\n", p.ID)
}

// Run runs a philosopher for a number of cycles
func (dp *DiningPhilosophers) Run(meals int) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(len(dp.Philosophers))

	for _, p := range dp.Philosophers {
		go func(phil *Philosopher) {
			defer wg.Done()
			for i := 0; i < meals; i++ {
				phil.Think()
				phil.Eat(dp.MaxEaters)
			}
		}(p)
	}

	return &wg
}

// GetMealsEaten returns total meals eaten by all philosophers
func (dp *DiningPhilosophers) GetMealsEaten() int {
	total := 0
	for _, p := range dp.Philosophers {
		p.mu.Lock()
		total += p.MealsEaten
		p.mu.Unlock()
	}
	return total
}

// =============================================================================
// Readers-Writers Problem
// =============================================================================

// ReadersWriters implements the readers-writers problem
type ReadersWriters struct {
	data          int
	readers       int
	writers       int
	readCount     int
	writeCount    int
	mu            sync.Mutex
	readerCond    *sync.Cond
	writerCond    *sync.Cond
	writerPref    bool // True for writer preference, false for reader preference
	totalReads    int
	totalWrites   int
}

// NewReadersWriters creates a new readers-writers instance
func NewReadersWriters(writerPreference bool) *ReadersWriters {
	rw := &ReadersWriters{
		data:       0,
		writerPref: writerPreference,
	}
	rw.readerCond = sync.NewCond(&rw.mu)
	rw.writerCond = sync.NewCond(&rw.mu)
	return rw
}

// StartRead acquires read lock
func (rw *ReadersWriters) StartRead(readerID int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.writerPref {
		// Wait if there are writers or waiting writers
		for rw.writers > 0 || rw.writeCount > 0 {
			rw.readerCond.Wait()
		}
	} else {
		// Wait only if there are active writers
		for rw.writers > 0 {
			rw.readerCond.Wait()
		}
	}

	rw.readers++
	rw.readCount++
}

// Read performs the read operation
func (rw *ReadersWriters) Read(readerID int) int {
	rw.StartRead(readerID)
	defer rw.EndRead(readerID)

	fmt.Printf("Reader %d: Reading data = %d\n", readerID, rw.data)
	time.Sleep(time.Millisecond * 50)

	rw.mu.Lock()
	rw.totalReads++
	rw.mu.Unlock()

	return rw.data
}

// EndRead releases read lock
func (rw *ReadersWriters) EndRead(readerID int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	rw.readers--
	if rw.readers == 0 {
		rw.writerCond.Signal()
	}
}

// StartWrite acquires write lock
func (rw *ReadersWriters) StartWrite(writerID int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	rw.writeCount++
	for rw.readers > 0 || rw.writers > 0 {
		rw.writerCond.Wait()
	}
	rw.writeCount--
	rw.writers++
}

// Write performs the write operation
func (rw *ReadersWriters) Write(writerID int, value int) {
	rw.StartWrite(writerID)
	defer rw.EndWrite(writerID)

	fmt.Printf("Writer %d: Writing data = %d\n", writerID, value)
	rw.data = value
	time.Sleep(time.Millisecond * 100)

	rw.mu.Lock()
	rw.totalWrites++
	rw.mu.Unlock()
}

// EndWrite releases write lock
func (rw *ReadersWriters) EndWrite(writerID int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	rw.writers--
	if rw.writerPref && rw.writeCount > 0 {
		rw.writerCond.Signal()
	} else {
		rw.readerCond.Broadcast()
		rw.writerCond.Signal()
	}
}

// RunReaders runs multiple reader goroutines
func (rw *ReadersWriters) RunReaders(numReaders, readsPerReader int) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(numReaders)

	for i := 0; i < numReaders; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < readsPerReader; j++ {
				rw.Read(id)
				time.Sleep(time.Millisecond * 30)
			}
		}(i)
	}

	return &wg
}

// RunWriters runs multiple writer goroutines
func (rw *ReadersWriters) RunWriters(numWriters, writesPerWriter int) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				value := id*100 + j
				rw.Write(id, value)
				time.Sleep(time.Millisecond * 80)
			}
		}(i)
	}

	return &wg
}

// GetStats returns read/write statistics
func (rw *ReadersWriters) GetStats() (reads, writes int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.totalReads, rw.totalWrites
}

// =============================================================================
// Sleeping Barber Problem
// =============================================================================

// SleepingBarber implements the sleeping barber problem
type SleepingBarber struct {
	numChairs   int
	waiting     []int // Queue of waiting customers
	barberReady chan struct{}
	customer    chan int
	done        chan struct{}
	totalCuts   int
	totalLeft   int
	mu          sync.Mutex
}

// NewSleepingBarber creates a new sleeping barber instance
func NewSleepingBarber(numChairs int) *SleepingBarber {
	return &SleepingBarber{
		numChairs:   numChairs,
		waiting:     make([]int, 0, numChairs),
		barberReady: make(chan struct{}, 1),
		customer:    make(chan int),
		done:        make(chan struct{}),
	}
}

// RunBarber runs the barber goroutine
func (sb *SleepingBarber) RunBarber() {
	go func() {
		for {
			select {
			case <-sb.done:
				return
			case customerID := <-sb.customer:
				fmt.Printf("Barber: Cutting hair of customer %d\n", customerID)
				time.Sleep(time.Millisecond * 200) // Haircut time
				fmt.Printf("Barber: Finished customer %d\n", customerID)

				sb.mu.Lock()
				sb.totalCuts++
				sb.mu.Unlock()

				// Signal barber is ready for next customer
				sb.barberReady <- struct{}{}
			default:
				fmt.Println("Barber: Sleeping (no customers)")
				// Wait for customer
				customerID := <-sb.customer
				fmt.Printf("Barber: Woken up by customer %d\n", customerID)
				fmt.Printf("Barber: Cutting hair of customer %d\n", customerID)
				time.Sleep(time.Millisecond * 200)
				fmt.Printf("Barber: Finished customer %d\n", customerID)

				sb.mu.Lock()
				sb.totalCuts++
				sb.mu.Unlock()

				sb.barberReady <- struct{}{}
			}
		}
	}()
}

// Customer simulates a customer arriving
func (sb *SleepingBarber) Customer(customerID int) {
	sb.mu.Lock()

	if len(sb.waiting) < sb.numChairs {
		// Take a seat
		sb.waiting = append(sb.waiting, customerID)
		fmt.Printf("Customer %d: Waiting in chair (waiting: %d)\n", customerID, len(sb.waiting))
		sb.mu.Unlock()

		// Notify barber
		sb.customer <- customerID

		// Wait for barber to finish
		<-sb.barberReady

		// Leave
		sb.mu.Lock()
		sb.waiting = sb.waiting[1:]
		sb.mu.Unlock()

		fmt.Printf("Customer %d: Done, leaving\n", customerID)
	} else {
		// No chairs available, leave
		fmt.Printf("Customer %d: No chairs available, leaving\n", customerID)
		sb.totalLeft++
		sb.mu.Unlock()
	}
}

// RunCustomers simulates customers arriving
func (sb *SleepingBarber) RunCustomers(numCustomers int, arrivalDelay time.Duration) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(numCustomers)

	for i := 0; i < numCustomers; i++ {
		go func(id int) {
			defer wg.Done()
			time.Sleep(arrivalDelay * time.Duration(id))
			sb.Customer(id)
		}(i)
	}

	return &wg
}

// Stop stops the barber
func (sb *SleepingBarber) Stop() {
	close(sb.done)
}

// GetStats returns statistics
func (sb *SleepingBarber) GetStats() (cuts, left int) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.totalCuts, sb.totalLeft
}

// =============================================================================
// Cigarette Smokers Problem
// =============================================================================

// Ingredient represents a smoking ingredient
type Ingredient int

const (
	Tobacco Ingredient = iota
	Paper
	Match
)

func (i Ingredient) String() string {
	return [...]string{"Tobacco", "Paper", "Match"}[i]
}

// CigaretteSmokers implements the cigarette smokers problem
type CigaretteSmokers struct {
	table         map[Ingredient]bool
	mu            sync.Mutex
	smokerConds   map[Ingredient]*sync.Cond
	agentCond     *sync.Cond
	totalSmokes   int
}

// NewCigaretteSmokers creates a new cigarette smokers instance
func NewCigaretteSmokers() *CigaretteSmokers {
	cs := &CigaretteSmokers{
		table:       make(map[Ingredient]bool),
		smokerConds: make(map[Ingredient]*sync.Cond),
	}
	cs.agentCond = sync.NewCond(&cs.mu)

	// Create condition variable for each smoker type
	for i := Tobacco; i <= Match; i++ {
		cs.smokerConds[i] = sync.NewCond(&cs.mu)
	}

	return cs
}

// Agent places two ingredients on table
func (cs *CigaretteSmokers) Agent(ing1, ing2 Ingredient) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Wait for table to be empty
	for len(cs.table) > 0 {
		cs.agentCond.Wait()
	}

	// Place ingredients
	cs.table[ing1] = true
	cs.table[ing2] = true
	fmt.Printf("Agent: Placed %s and %s on table\n", ing1, ing2)

	// Determine which smoker can smoke and wake them
	var smokerType Ingredient
	if !cs.table[Tobacco] {
		smokerType = Tobacco
	} else if !cs.table[Paper] {
		smokerType = Paper
	} else {
		smokerType = Match
	}

	cs.smokerConds[smokerType].Signal()
}

// Smoker smokes a cigarette
func (cs *CigaretteSmokers) Smoker(has Ingredient, smokerID int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Wait for the right ingredients
	for {
		// Check if the other two ingredients are available
		count := 0
		for ing := range cs.table {
			if ing != has {
				count++
			}
		}

		if count == 2 {
			break
		}

		cs.smokerConds[has].Wait()
	}

	// Take ingredients and smoke
	fmt.Printf("Smoker %d (has %s): Taking ingredients and smoking\n", smokerID, has)
	cs.table = make(map[Ingredient]bool) // Clear table
	cs.totalSmokes++

	// Signal agent
	cs.agentCond.Signal()
}

// GetStats returns total smokes
func (cs *CigaretteSmokers) GetStats() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.totalSmokes
}

// =============================================================================
// Bounded Buffer with Multiple Producers and Consumers
// =============================================================================

// BoundedBuffer implements a thread-safe bounded buffer
type BoundedBuffer struct {
	buffer   []interface{}
	capacity int
	size     int
	head     int
	tail     int
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	itemsIn  int
	itemsOut int
}

// NewBoundedBuffer creates a bounded buffer
func NewBoundedBuffer(capacity int) *BoundedBuffer {
	bb := &BoundedBuffer{
		buffer:   make([]interface{}, capacity),
		capacity: capacity,
		size:     0,
		head:     0,
		tail:     0,
	}
	bb.notEmpty = sync.NewCond(&bb.mu)
	bb.notFull = sync.NewCond(&bb.mu)
	return bb
}

// Put adds an item to the buffer
func (bb *BoundedBuffer) Put(item interface{}) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	for bb.size == bb.capacity {
		bb.notFull.Wait()
	}

	bb.buffer[bb.tail] = item
	bb.tail = (bb.tail + 1) % bb.capacity
	bb.size++
	bb.itemsIn++

	bb.notEmpty.Signal()
}

// Get removes and returns an item from the buffer
func (bb *BoundedBuffer) Get() interface{} {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	for bb.size == 0 {
		bb.notEmpty.Wait()
	}

	item := bb.buffer[bb.head]
	bb.head = (bb.head + 1) % bb.capacity
	bb.size--
	bb.itemsOut++

	bb.notFull.Signal()

	return item
}

// GetStats returns buffer statistics
func (bb *BoundedBuffer) GetStats() (in, out, current int) {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	return bb.itemsIn, bb.itemsOut, bb.size
}
