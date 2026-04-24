package ipc

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// IPC-001: Named Pipes (FIFOs) Tests
// =============================================================================

func TestFIFO_CreateOpenReadWrite(t *testing.T) {
	reg := NewPipeRegistry()
	if err := reg.Create("test", 8); err != nil {
		t.Fatal(err)
	}

	var np *NamedPipe
	var writeErr error

	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		np, err = reg.OpenRead("test")
		if err != nil {
			t.Errorf("OpenRead: %v", err)
		}
	}()

	wp, err := reg.OpenWrite("test")
	if err != nil {
		t.Fatalf("OpenWrite: %v", err)
	}
	_ = writeErr
	<-done

	wp.Write([]byte("hello"))
	data := np.Read()
	if string(data) != "hello" {
		t.Errorf("expected hello, got %q", data)
	}
}

func TestFIFO_BlocksUntilBothSides(t *testing.T) {
	reg := NewPipeRegistry()
	reg.Create("block-test", 4)

	writerReady := make(chan struct{})
	go func() {
		close(writerReady)
		reg.OpenWrite("block-test") // blocks until reader connects
	}()
	<-writerReady
	// Small delay to ensure writer is blocking.
	time.Sleep(5 * time.Millisecond)
	reg.OpenRead("block-test") // unblocks writer
}

func TestFIFO_DuplicateName(t *testing.T) {
	reg := NewPipeRegistry()
	reg.Create("dup", 4)
	if err := reg.Create("dup", 4); err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestFIFO_NotFound(t *testing.T) {
	reg := NewPipeRegistry()
	if _, err := reg.OpenRead("missing"); err == nil {
		t.Error("expected error for missing pipe")
	}
}

func TestFIFO_MultipleMessages(t *testing.T) {
	reg := NewPipeRegistry()
	reg.Create("multi", 16)

	var np *NamedPipe
	ready := make(chan struct{})
	go func() {
		var err error
		np, err = reg.OpenRead("multi")
		if err != nil {
			t.Errorf("OpenRead: %v", err)
		}
		close(ready)
	}()

	wp, _ := reg.OpenWrite("multi")
	<-ready

	for i := 0; i < 5; i++ {
		wp.Write([]byte{byte(i)})
	}
	for i := 0; i < 5; i++ {
		data := np.Read()
		if data[0] != byte(i) {
			t.Errorf("expected %d, got %d", i, data[0])
		}
	}
}

func TestFIFO_MultipleReaders(t *testing.T) {
	reg := NewPipeRegistry()
	reg.Create("multi-reader", 32)

	var readers [3]*NamedPipe
	var wg sync.WaitGroup
	for i := range readers {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := reg.OpenRead("multi-reader")
			if err != nil {
				t.Errorf("OpenRead[%d]: %v", i, err)
				return
			}
			readers[i] = r
		}()
	}

	wp, _ := reg.OpenWrite("multi-reader")
	wg.Wait()

	if wp.Readers() < 3 {
		t.Errorf("expected at least 3 readers, got %d", wp.Readers())
	}
}

func TestFIFO_TryRead(t *testing.T) {
	reg := NewPipeRegistry()
	reg.Create("try", 4)

	var np *NamedPipe
	ready := make(chan struct{})
	go func() {
		var err error
		np, err = reg.OpenRead("try")
		if err != nil {
			t.Errorf("OpenRead: %v", err)
		}
		close(ready)
	}()

	wp, _ := reg.OpenWrite("try")
	<-ready

	if _, ok := np.TryRead(); ok {
		t.Error("TryRead on empty pipe should return false")
	}
	wp.Write([]byte("data"))
	time.Sleep(time.Millisecond)
	if data, ok := np.TryRead(); !ok || string(data) != "data" {
		t.Errorf("expected data, got %q ok=%v", data, ok)
	}
}

// =============================================================================
// IPC-002: Pub-Sub Broker Tests
// =============================================================================

func TestPubSub_SubscribePublish(t *testing.T) {
	b := NewPubSubBroker()
	ch := b.Subscribe("news", 4)
	b.Publish("news", "breaking!")
	msg := <-ch
	if msg != "breaking!" {
		t.Errorf("expected breaking!, got %v", msg)
	}
}

func TestPubSub_MultipleSubscribers(t *testing.T) {
	b := NewPubSubBroker()
	ch1 := b.Subscribe("topic", 4)
	ch2 := b.Subscribe("topic", 4)
	b.Publish("topic", 42)
	if v := <-ch1; v != 42 {
		t.Errorf("ch1: expected 42, got %v", v)
	}
	if v := <-ch2; v != 42 {
		t.Errorf("ch2: expected 42, got %v", v)
	}
}

func TestPubSub_Backpressure(t *testing.T) {
	b := NewPubSubBroker()
	_ = b.Subscribe("slow", 2)
	// Fill the buffer, then overflow.
	b.Publish("slow", 1)
	b.Publish("slow", 2)
	dropped := b.Publish("slow", 3) // should be dropped
	if dropped == 0 {
		t.Error("expected a drop when subscriber channel is full")
	}
	if b.TotalDropped == 0 {
		t.Error("expected TotalDropped > 0")
	}
}

func TestPubSub_NoSubscribers(t *testing.T) {
	b := NewPubSubBroker()
	dropped := b.Publish("ghost", "msg")
	if dropped != 0 {
		t.Errorf("no subscribers means no drops, got %d", dropped)
	}
	if b.TotalPublished != 1 {
		t.Errorf("expected 1 published, got %d", b.TotalPublished)
	}
}

func TestPubSub_TopicIsolation(t *testing.T) {
	b := NewPubSubBroker()
	ch := b.Subscribe("A", 4)
	b.Subscribe("B", 4)
	b.Publish("B", "for B only")
	select {
	case <-ch:
		t.Error("ch for topic A should not receive messages for topic B")
	default:
	}
}

func TestPubSub_SubscriberCount(t *testing.T) {
	b := NewPubSubBroker()
	b.Subscribe("t", 4)
	b.Subscribe("t", 4)
	if b.SubscriberCount("t") != 2 {
		t.Errorf("expected 2 subscribers, got %d", b.SubscriberCount("t"))
	}
}

func TestPubSub_Concurrent(t *testing.T) {
	b := NewPubSubBroker()
	for i := 0; i < 8; i++ {
		b.Subscribe("flood", 256)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Publish("flood", "msg")
		}()
	}
	wg.Wait()
}

// =============================================================================
// IPC-003: Ordered Message Passing Tests
// =============================================================================

func TestOrdered_FIFO(t *testing.T) {
	om := NewOrderedMessenger()
	om.Register(1)
	om.Register(2)

	// Process 1 sends 5 messages — should arrive in order at process 2.
	for i := 0; i < 5; i++ {
		om.Send(1, i)
	}

	msgs, err := om.Receive(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	for i, m := range msgs {
		if m.Data.(int) != i {
			t.Errorf("FIFO violated at position %d: expected %d, got %v", i, i, m.Data)
		}
	}
}

func TestOrdered_LamportTimestamps(t *testing.T) {
	om := NewOrderedMessenger()
	om.Register(1)
	om.Register(2)

	om.Send(1, "first")
	om.Send(2, "second")
	om.Send(1, "third")

	msgs, _ := om.Receive(1)
	for i := 1; i < len(msgs); i++ {
		if msgs[i].LamportTS < msgs[i-1].LamportTS {
			t.Errorf("Lamport order violated: msg[%d].TS=%d < msg[%d].TS=%d",
				i, msgs[i].LamportTS, i-1, msgs[i-1].LamportTS)
		}
	}
}

func TestOrdered_VectorClock(t *testing.T) {
	om := NewOrderedMessenger()
	om.Register(10)
	om.Register(20)

	om.Send(10, "a")
	om.Send(20, "b")

	msgs10, _ := om.Receive(10)
	msgs20, _ := om.Receive(20)

	for _, m := range append(msgs10, msgs20...) {
		if m.VectorTS == nil {
			t.Error("vector clock should be set on all messages")
		}
	}
}

func TestOrdered_UnregisteredReceiver(t *testing.T) {
	om := NewOrderedMessenger()
	if _, err := om.Receive(999); err == nil {
		t.Error("expected error for unregistered receiver")
	}
}

func TestOrdered_BroadcastAllReceive(t *testing.T) {
	om := NewOrderedMessenger()
	for i := 1; i <= 5; i++ {
		om.Register(i)
	}
	om.Send(1, "broadcast")
	for i := 1; i <= 5; i++ {
		msgs, _ := om.Receive(i)
		if len(msgs) != 1 || msgs[0].Data != "broadcast" {
			t.Errorf("process %d did not receive broadcast: %v", i, msgs)
		}
	}
}

func TestOrdered_Concurrent(t *testing.T) {
	om := NewOrderedMessenger()
	for i := 1; i <= 8; i++ {
		om.Register(i)
	}
	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				om.Send(i, j)
			}
		}()
	}
	wg.Wait()
	// All receivers should have messages.
	for i := 1; i <= 8; i++ {
		msgs, _ := om.Receive(i)
		if len(msgs) == 0 {
			t.Errorf("process %d received no messages", i)
		}
	}
}
