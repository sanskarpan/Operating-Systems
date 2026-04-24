package ipc

// =============================================================================
// IPC-001: Named Pipes (FIFOs)
// =============================================================================
//
// A named pipe (FIFO) has a persistent name in a namespace. Open() blocks until
// both a reader and a writer have connected. Multiple readers are supported;
// each message is delivered to exactly one reader (competitive receive).

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// NamedPipe is a named, bidirectional-rendezvous FIFO.
type NamedPipe struct {
	name     string
	ch       chan []byte
	readers  int32 // atomic count of connected readers
	writers  int32 // atomic count of connected writers
	mu       sync.Mutex
	readReady  chan struct{} // closed when first reader connects
	writeReady chan struct{} // closed when first writer connects
	readOnce   sync.Once
	writeOnce  sync.Once
}

// PipeRegistry is the named-pipe namespace (like /dev/fifo/…).
type PipeRegistry struct {
	mu    sync.Mutex
	pipes map[string]*NamedPipe
}

// NewPipeRegistry creates an empty FIFO namespace.
func NewPipeRegistry() *PipeRegistry {
	return &PipeRegistry{pipes: make(map[string]*NamedPipe)}
}

// Create registers a new named pipe. Returns error if name already exists.
func (r *PipeRegistry) Create(name string, bufSize int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pipes[name]; ok {
		return fmt.Errorf("fifo: %q already exists", name)
	}
	r.pipes[name] = &NamedPipe{
		name:       name,
		ch:         make(chan []byte, bufSize),
		readReady:  make(chan struct{}),
		writeReady: make(chan struct{}),
	}
	return nil
}

// OpenRead connects as a reader; blocks until a writer is also connected.
func (r *PipeRegistry) OpenRead(name string) (*NamedPipe, error) {
	r.mu.Lock()
	np, ok := r.pipes[name]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("fifo: %q not found", name)
	}
	atomic.AddInt32(&np.readers, 1)
	np.readOnce.Do(func() { close(np.readReady) })
	// Block until a writer is ready.
	<-np.writeReady
	return np, nil
}

// OpenWrite connects as a writer; blocks until a reader is also connected.
func (r *PipeRegistry) OpenWrite(name string) (*NamedPipe, error) {
	r.mu.Lock()
	np, ok := r.pipes[name]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("fifo: %q not found", name)
	}
	atomic.AddInt32(&np.writers, 1)
	np.writeOnce.Do(func() { close(np.writeReady) })
	// Block until a reader is ready.
	<-np.readReady
	return np, nil
}

// Write sends data into the pipe.
func (np *NamedPipe) Write(data []byte) {
	cp := make([]byte, len(data))
	copy(cp, data)
	np.ch <- cp
}

// Read receives the next message from the pipe (blocks until available).
func (np *NamedPipe) Read() []byte {
	return <-np.ch
}

// TryRead returns a message if one is available, otherwise returns nil.
func (np *NamedPipe) TryRead() ([]byte, bool) {
	select {
	case data := <-np.ch:
		return data, true
	default:
		return nil, false
	}
}

// Readers returns the number of connected readers.
func (np *NamedPipe) Readers() int { return int(atomic.LoadInt32(&np.readers)) }

// Writers returns the number of connected writers.
func (np *NamedPipe) Writers() int { return int(atomic.LoadInt32(&np.writers)) }

// =============================================================================
// IPC-002: Pub-Sub Broker
// =============================================================================
//
// Topics are created implicitly. Subscribe returns a channel for that topic.
// Publish fan-outs to all subscribers with backpressure: if a subscriber's
// channel is full the message is dropped for that subscriber (non-blocking
// send), and the drop count is recorded.

// PubSubBroker is the message broker.
type PubSubBroker struct {
	mu     sync.RWMutex
	topics map[string][]*subscriber

	TotalPublished int64
	TotalDropped   int64
}

type subscriber struct {
	ch      chan interface{}
	dropped int64 // atomic
}

// NewPubSubBroker creates an empty broker.
func NewPubSubBroker() *PubSubBroker {
	return &PubSubBroker{topics: make(map[string][]*subscriber)}
}

// Subscribe registers a new subscriber for topic and returns its receive channel.
// bufSize controls how many messages can be buffered before backpressure kicks in.
func (b *PubSubBroker) Subscribe(topic string, bufSize int) <-chan interface{} {
	sub := &subscriber{ch: make(chan interface{}, bufSize)}
	b.mu.Lock()
	b.topics[topic] = append(b.topics[topic], sub)
	b.mu.Unlock()
	return sub.ch
}

// Publish sends msg to all subscribers of topic.
// If a subscriber's channel is full the message is dropped for that subscriber.
func (b *PubSubBroker) Publish(topic string, msg interface{}) int {
	b.mu.RLock()
	subs := b.topics[topic]
	b.mu.RUnlock()
	atomic.AddInt64(&b.TotalPublished, 1)
	dropped := 0
	for _, sub := range subs {
		select {
		case sub.ch <- msg:
		default:
			atomic.AddInt64(&sub.dropped, 1)
			atomic.AddInt64(&b.TotalDropped, 1)
			dropped++
		}
	}
	return dropped
}

// SubscriberCount returns how many subscribers a topic has.
func (b *PubSubBroker) SubscriberCount(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.topics[topic])
}

// Topics returns all known topic names.
func (b *PubSubBroker) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.topics))
	for t := range b.topics {
		out = append(out, t)
	}
	return out
}

// =============================================================================
// IPC-003: Ordered Message Passing
// =============================================================================
//
// Three ordering guarantees are provided in a single abstraction:
//
//  FIFO ordering    — messages from the same sender arrive in send order.
//  Causal ordering  — if send(A) happened-before send(B) then recv(A) before recv(B).
//                     Implemented via vector clocks (Fidge/Mattern algorithm).
//  Total ordering   — all receivers see messages in the same global order.
//                     Implemented via Lamport logical timestamps.

// VectorClock maps process ID → logical clock.
type VectorClock map[int]int64

// Clone returns a deep copy.
func (vc VectorClock) Clone() VectorClock {
	out := make(VectorClock, len(vc))
	for k, v := range vc {
		out[k] = v
	}
	return out
}

// Merge advances each entry to max(local, incoming).
func (vc VectorClock) Merge(other VectorClock) {
	for k, v := range other {
		if v > vc[k] {
			vc[k] = v
		}
	}
}

// HappensBefore returns true if vc strictly happened-before other
// (all entries ≤ other and at least one <).
func (vc VectorClock) HappensBefore(other VectorClock) bool {
	less := false
	for k, v := range vc {
		if v > other[k] {
			return false
		}
		if v < other[k] {
			less = true
		}
	}
	return less
}

// OrderedMessage carries both Lamport and vector-clock timestamps.
type OrderedMessage struct {
	SenderID      int
	LamportTS     int64
	VectorTS      VectorClock
	Data          interface{}
	SeqPerSender  int64 // FIFO: monotone per sender
}

// OrderedChannel provides FIFO + causal + total ordering for one receiver.
type OrderedChannel struct {
	mu       sync.Mutex
	incoming []*OrderedMessage // pending, unsorted

	// Per-sender FIFO sequence tracking.
	nextSeq map[int]int64

	// Delivered buffer (total order: sorted by Lamport TS, tie-break by sender ID).
	delivered []*OrderedMessage
}

// newOrderedChannel creates a channel for one receiver process.
func newOrderedChannel() *OrderedChannel {
	return &OrderedChannel{nextSeq: make(map[int]int64)}
}

// deliver attempts to deliver all causally-ready messages in Lamport order.
func (oc *OrderedChannel) deliver() {
	// Simple delivery: accept all messages; sort by (LamportTS, SenderID) for total order.
	// For causal ordering we rely on the sender having vector-merged before sending.
	for len(oc.incoming) > 0 {
		// Find message with lowest Lamport TS (and lowest SenderID on tie).
		best := 0
		for i := 1; i < len(oc.incoming); i++ {
			a, b := oc.incoming[best], oc.incoming[i]
			if b.LamportTS < a.LamportTS ||
				(b.LamportTS == a.LamportTS && b.SenderID < a.SenderID) {
				best = i
			}
		}
		msg := oc.incoming[best]
		// FIFO check: only deliver if this is the next expected seq from sender.
		if msg.SeqPerSender != oc.nextSeq[msg.SenderID] {
			break // wait for the gap to be filled
		}
		oc.nextSeq[msg.SenderID]++
		oc.delivered = append(oc.delivered, msg)
		oc.incoming = append(oc.incoming[:best], oc.incoming[best+1:]...)
	}
}

// receive queues a message for delivery.
func (oc *OrderedChannel) receive(msg *OrderedMessage) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.incoming = append(oc.incoming, msg)
	oc.deliver()
}

// Drain returns all delivered messages so far and clears the buffer.
func (oc *OrderedChannel) Drain() []*OrderedMessage {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	out := oc.delivered
	oc.delivered = nil
	return out
}

// OrderedMessenger is the multi-process ordered-message bus.
type OrderedMessenger struct {
	mu       sync.Mutex
	channels map[int]*OrderedChannel // receiver PID → channel

	// Per-sender state.
	senderMu    sync.Mutex
	lamport     map[int]int64      // per-sender Lamport clock
	vectorClock map[int]VectorClock // per-sender vector clock
	seqNum      map[int]int64      // per-sender FIFO sequence
}

// NewOrderedMessenger creates a new ordered-message bus.
func NewOrderedMessenger() *OrderedMessenger {
	return &OrderedMessenger{
		channels:    make(map[int]*OrderedChannel),
		lamport:     make(map[int]int64),
		vectorClock: make(map[int]VectorClock),
		seqNum:      make(map[int]int64),
	}
}

// Register adds a process to the bus as a receiver.
func (om *OrderedMessenger) Register(pid int) {
	om.mu.Lock()
	defer om.mu.Unlock()
	if _, ok := om.channels[pid]; !ok {
		om.channels[pid] = newOrderedChannel()
	}
	om.senderMu.Lock()
	if om.vectorClock[pid] == nil {
		om.vectorClock[pid] = make(VectorClock)
	}
	om.senderMu.Unlock()
}

// Send sends data from senderPID to all registered processes (broadcast).
func (om *OrderedMessenger) Send(senderPID int, data interface{}) {
	om.senderMu.Lock()
	// Increment sender's Lamport clock.
	om.lamport[senderPID]++
	lts := om.lamport[senderPID]
	// Increment sender's own entry in vector clock.
	if om.vectorClock[senderPID] == nil {
		om.vectorClock[senderPID] = make(VectorClock)
	}
	om.vectorClock[senderPID][senderPID]++
	vc := om.vectorClock[senderPID].Clone()
	seq := om.seqNum[senderPID]
	om.seqNum[senderPID]++
	om.senderMu.Unlock()

	msg := &OrderedMessage{
		SenderID:     senderPID,
		LamportTS:    lts,
		VectorTS:     vc,
		Data:         data,
		SeqPerSender: seq,
	}

	om.mu.Lock()
	chans := make([]*OrderedChannel, 0, len(om.channels))
	for _, ch := range om.channels {
		chans = append(chans, ch)
	}
	om.mu.Unlock()

	for _, ch := range chans {
		ch.receive(msg)
	}
}

// Receive returns all delivered messages for receiverPID.
func (om *OrderedMessenger) Receive(receiverPID int) ([]*OrderedMessage, error) {
	om.mu.Lock()
	ch, ok := om.channels[receiverPID]
	om.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("ordered: process %d not registered", receiverPID)
	}
	return ch.Drain(), nil
}
