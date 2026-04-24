package concurrency

import (
	"sync"
	"testing"
	"time"
)

func TestProducerConsumer(t *testing.T) {
	pc := NewProducerConsumer(5)

	var wg sync.WaitGroup
	itemsProduced := 0
	itemsConsumed := 0
	var mu sync.Mutex

	// Start producers
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				pc.Produce(id*100+j, id)
				mu.Lock()
				itemsProduced++
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Start consumers
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				pc.Consume(id)
				mu.Lock()
				itemsConsumed++
				mu.Unlock()
				time.Sleep(15 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	if itemsProduced != 10 {
		t.Errorf("Expected 10 items produced, got %d", itemsProduced)
	}

	if itemsConsumed != 10 {
		t.Errorf("Expected 10 items consumed, got %d", itemsConsumed)
	}
}

func TestDiningPhilosophers(t *testing.T) {
	dp := NewDiningPhilosophers(5)

	wg := dp.Run(2)
	wg.Wait()

	totalMeals := dp.GetMealsEaten()
	expected := 5 * 2 // 5 philosophers, 2 meals each

	if totalMeals != expected {
		t.Errorf("Expected %d total meals, got %d", expected, totalMeals)
	}
}

func TestDiningPhilosophersNoDeadlock(t *testing.T) {
	dp := NewDiningPhilosophers(3)

	done := make(chan bool)
	go func() {
		wg := dp.Run(3)
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success - no deadlock
		t.Log("No deadlock detected")
	case <-time.After(5 * time.Second):
		t.Error("Dining philosophers deadlocked")
	}
}

func TestReadersWriters(t *testing.T) {
	rw := NewReadersWriters(false)

	var wg sync.WaitGroup

	// Start readers
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				rw.Read(id)
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Start writers
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				rw.Write(id, id*100+j)
				time.Sleep(20 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	reads, writes := rw.GetStats()

	if reads != 9 {
		t.Errorf("Expected 9 reads, got %d", reads)
	}

	if writes != 6 {
		t.Errorf("Expected 6 writes, got %d", writes)
	}
}

func TestReadersWritersConcurrency(t *testing.T) {
	rw := NewReadersWriters(false)

	// Start multiple readers simultaneously
	var readersActive sync.WaitGroup
	readersActive.Add(3)

	started := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			rw.StartRead(id)
			started <- true
			readersActive.Done()
			time.Sleep(100 * time.Millisecond)
			rw.EndRead(id)
		}(i)
	}

	// Wait for all readers to start
	<-started
	<-started
	<-started

	// All 3 readers should be active concurrently
	readersActive.Wait()
}

func TestSleepingBarber(t *testing.T) {
	sb := NewSleepingBarber(3)
	sb.RunBarber()

	// Send customers
	wg := sb.RunCustomers(5, 50*time.Millisecond)
	wg.Wait()

	time.Sleep(2 * time.Second) // Allow barber to finish

	sb.Stop()

	cuts, left := sb.GetStats()

	if cuts == 0 {
		t.Error("Expected some customers to get haircuts")
	}

	t.Logf("Customers served: %d, Customers left: %d", cuts, left)
}

func TestBoundedBufferConcurrent(t *testing.T) {
	bb := NewBoundedBuffer(10)

	var wg sync.WaitGroup

	// Producers
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				bb.Put(id*100 + j)
			}
		}(i)
	}

	// Consumers
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				bb.Get()
			}
		}()
	}

	wg.Wait()

	itemsIn, itemsOut, current := bb.GetStats()

	if itemsIn != 30 {
		t.Errorf("Expected 30 items in, got %d", itemsIn)
	}

	if itemsOut != 30 {
		t.Errorf("Expected 30 items out, got %d", itemsOut)
	}

	if current != 0 {
		t.Errorf("Expected buffer to be empty, has %d items", current)
	}
}

func TestProducerConsumerGetStats(t *testing.T) {
	pc := NewProducerConsumer(5)

	// Produce some items
	for i := 0; i < 5; i++ {
		pc.Produce(i, 0)
	}

	// Consume some items
	for i := 0; i < 3; i++ {
		pc.Consume(0)
	}

	produced, consumed := pc.GetStats()

	if produced != 5 {
		t.Errorf("Expected 5 produced, got %d", produced)
	}

	if consumed != 3 {
		t.Errorf("Expected 3 consumed, got %d", consumed)
	}
}

func TestReadersWritersPreference(t *testing.T) {
	// Test writer preference
	rwWriter := NewReadersWriters(true)

	// This test verifies that writer preference is set
	if !rwWriter.writerPref {
		t.Error("Writer preference should be true")
	}

	// Test reader preference
	rwReader := NewReadersWriters(false)

	if rwReader.writerPref {
		t.Error("Writer preference should be false for reader preference")
	}
}

func TestCigaretteSmokersGetStats(t *testing.T) {
	cs := NewCigaretteSmokers()

	stats := cs.GetStats()
	if stats != 0 {
		t.Errorf("Expected 0 smokes initially, got %d", stats)
	}
}

func TestPhilosopherThinkEat(t *testing.T) {
	dp := NewDiningPhilosophers(3)

	philosopher := dp.Philosophers[0]

	// Test think (should not panic)
	philosopher.Think()

	// Initial meals eaten should be 0
	philosopher.mu.Lock()
	meals := philosopher.MealsEaten
	philosopher.mu.Unlock()

	if meals != 0 {
		t.Errorf("Expected 0 meals initially, got %d", meals)
	}
}

func TestReadersWritersMultipleReaders(t *testing.T) {
	rw := NewReadersWriters(false)

	// Start 5 readers concurrently
	var wg sync.WaitGroup
	wg.Add(5)

	for i := 0; i < 5; i++ {
		go func(id int) {
			defer wg.Done()
			rw.Read(id)
		}(i)
	}

	wg.Wait()

	reads, _ := rw.GetStats()
	if reads != 5 {
		t.Errorf("Expected 5 reads, got %d", reads)
	}
}

func TestProducerConsumerBufferBounds(t *testing.T) {
	pc := NewProducerConsumer(3)

	// Fill buffer
	pc.Produce(1, 0)
	pc.Produce(2, 0)
	pc.Produce(3, 0)

	// Try to produce when full (should block)
	done := make(chan bool)
	go func() {
		pc.Produce(4, 0) // This will block
		done <- true
	}()

	// Give it time to block
	time.Sleep(100 * time.Millisecond)

	// Consume to unblock
	pc.Consume(0)

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("Producer did not unblock")
	}
}
