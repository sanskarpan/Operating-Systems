/*
Networking Simulation
======================

Protocol and networking algorithm implementations:
  - TCP finite state machine         (NET-001)
  - Sliding window flow control      (NET-002)
  - ARQ protocols                    (NET-003)
  - Nagle's algorithm                (NET-004)
  - Basic routing table              (NET-005)
*/

package networking

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
)

// =============================================================================
// NET-001: TCP Finite State Machine
// =============================================================================

// TCPState represents one of the 11 TCP connection states.
type TCPState int

const (
	StateClosed      TCPState = iota
	StateListen               // server waiting for connection
	StateSynSent              // client sent SYN
	StateSynReceived          // server received SYN, sent SYN-ACK
	StateEstablished          // connection open
	StateFinWait1             // active close: FIN sent
	StateFinWait2             // active close: FIN acked
	StateTimeWait             // waiting for duplicate packets to expire
	StateCloseWait            // passive close: received FIN
	StateLastAck              // passive close: FIN sent, waiting for final ACK
	StateClosing              // both sides closing simultaneously
)

func (s TCPState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateListen:
		return "LISTEN"
	case StateSynSent:
		return "SYN_SENT"
	case StateSynReceived:
		return "SYN_RECEIVED"
	case StateEstablished:
		return "ESTABLISHED"
	case StateFinWait1:
		return "FIN_WAIT_1"
	case StateFinWait2:
		return "FIN_WAIT_2"
	case StateTimeWait:
		return "TIME_WAIT"
	case StateCloseWait:
		return "CLOSE_WAIT"
	case StateLastAck:
		return "LAST_ACK"
	case StateClosing:
		return "CLOSING"
	default:
		return "UNKNOWN"
	}
}

// TCPEvent is an input event to the TCP state machine.
type TCPEvent int

const (
	EventPassiveOpen TCPEvent = iota // application calls listen()
	EventActiveOpen                  // application calls connect()
	EventSYN                         // SYN segment received
	EventSYNACK                      // SYN-ACK segment received
	EventACK                         // ACK segment received
	EventFIN                         // FIN segment received
	EventClose                       // application calls close()
	EventTimeout                     // TIME_WAIT 2MSL timer expires
)

func (e TCPEvent) String() string {
	switch e {
	case EventPassiveOpen:
		return "PASSIVE_OPEN"
	case EventActiveOpen:
		return "ACTIVE_OPEN"
	case EventSYN:
		return "SYN"
	case EventSYNACK:
		return "SYN_ACK"
	case EventACK:
		return "ACK"
	case EventFIN:
		return "FIN"
	case EventClose:
		return "CLOSE"
	case EventTimeout:
		return "TIMEOUT"
	default:
		return "UNKNOWN"
	}
}

// tcpTransition defines a state transition.
type tcpTransition struct {
	nextState TCPState
	action    string
}

// tcpTable is the complete TCP state transition table.
// Key: (currentState, event) → (nextState, action).
var tcpTable = map[TCPState]map[TCPEvent]tcpTransition{
	StateClosed: {
		EventPassiveOpen: {StateListen, "allocate TCB"},
		EventActiveOpen:  {StateSynSent, "send SYN"},
	},
	StateListen: {
		EventSYN:   {StateSynReceived, "send SYN-ACK"},
		EventClose: {StateClosed, "free TCB"},
	},
	StateSynSent: {
		EventSYNACK: {StateEstablished, "send ACK"},
		EventSYN:    {StateClosing, "send SYN-ACK (simultaneous open)"},
		EventClose:  {StateClosed, "free TCB"},
	},
	StateSynReceived: {
		EventACK:   {StateEstablished, "connection established"},
		EventClose: {StateFinWait1, "send FIN"},
	},
	StateEstablished: {
		EventClose: {StateFinWait1, "send FIN"},
		EventFIN:   {StateCloseWait, "send ACK"},
	},
	StateFinWait1: {
		EventACK: {StateFinWait2, "wait for FIN"},
		EventFIN: {StateClosing, "send ACK"},
	},
	StateFinWait2: {
		EventFIN: {StateTimeWait, "send ACK"},
	},
	StateTimeWait: {
		EventTimeout: {StateClosed, "delete TCB"},
	},
	StateCloseWait: {
		EventClose: {StateLastAck, "send FIN"},
	},
	StateLastAck: {
		EventACK: {StateClosed, "delete TCB"},
	},
	StateClosing: {
		EventACK: {StateTimeWait, "wait 2MSL"},
	},
}

// TCPConnection is a simulated TCP connection state machine.
type TCPConnection struct {
	mu      sync.Mutex
	state   TCPState
	History []string // log of transitions
}

// NewTCPConnection returns a connection starting in CLOSED state.
func NewTCPConnection() *TCPConnection {
	return &TCPConnection{state: StateClosed}
}

// State returns the current TCP state.
func (c *TCPConnection) State() TCPState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Event processes an event, advancing the state machine.
// Returns an error if the event is invalid for the current state.
func (c *TCPConnection) Event(ev TCPEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	transitions, ok := tcpTable[c.state]
	if !ok {
		return fmt.Errorf("no transitions defined from state %s", c.state)
	}
	tr, ok := transitions[ev]
	if !ok {
		return fmt.Errorf("invalid event %s in state %s", ev, c.state)
	}
	entry := fmt.Sprintf("%s -[%s]-> %s (%s)", c.state, ev, tr.nextState, tr.action)
	c.History = append(c.History, entry)
	c.state = tr.nextState
	return nil
}

// =============================================================================
// NET-002: Sliding Window Flow Control
// =============================================================================

// Segment represents a data segment in the sliding window protocol.
type Segment struct {
	SeqNum int
	Data   []byte
	Acked  bool
}

// SlidingWindowSender implements a sliding window sender.
type SlidingWindowSender struct {
	mu         sync.Mutex
	windowSize int       // max unacked segments
	base       int       // oldest unacked sequence number
	nextSeq    int       // next sequence number to send
	window     []*Segment
	lossRate   float64   // probability that a segment is "lost"
	rng        *rand.Rand

	// Metrics
	TotalSent      int64
	TotalRetrans   int64
	TotalAcked     int64
}

// NewSlidingWindowSender creates a sender with the given window size and loss rate.
func NewSlidingWindowSender(windowSize int, lossRate float64, seed int64) *SlidingWindowSender {
	return &SlidingWindowSender{
		windowSize: windowSize,
		window:     make([]*Segment, 0, windowSize),
		lossRate:   lossRate,
		rng:        rand.New(rand.NewSource(seed)),
	}
}

// Send attempts to send data; returns (seqNum, dropped) where dropped=true if simulated loss.
func (s *SlidingWindowSender) Send(data []byte) (seqNum int, dropped bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.window) >= s.windowSize {
		return 0, false, errors.New("send window full")
	}
	seg := &Segment{SeqNum: s.nextSeq, Data: data}
	s.window = append(s.window, seg)
	seqNum = s.nextSeq
	s.nextSeq++
	atomic.AddInt64(&s.TotalSent, 1)

	dropped = s.rng.Float64() < s.lossRate
	return seqNum, dropped, nil
}

// Ack processes an ACK for the given cumulative sequence number.
// All segments with seqNum ≤ ackNum are considered acknowledged.
func (s *SlidingWindowSender) Ack(ackNum int) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for len(s.window) > 0 && s.window[0].SeqNum <= ackNum {
		s.window = s.window[1:]
		s.base++
		removed++
		atomic.AddInt64(&s.TotalAcked, 1)
	}
	return removed
}

// Retransmit marks a segment as needing retransmission and returns it.
func (s *SlidingWindowSender) Retransmit(seqNum int) (*Segment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, seg := range s.window {
		if seg.SeqNum == seqNum {
			atomic.AddInt64(&s.TotalRetrans, 1)
			return seg, nil
		}
	}
	return nil, fmt.Errorf("segment %d not in window", seqNum)
}

// WindowUsed returns the number of unacknowledged segments.
func (s *SlidingWindowSender) WindowUsed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.window)
}

// SlidingWindowReceiver receives segments and sends cumulative ACKs.
type SlidingWindowReceiver struct {
	mu       sync.Mutex
	expected int       // next expected sequence number
	rcvWin   int       // receive window size
	buffer   map[int]*Segment // out-of-order buffer
	Received int64
}

// NewSlidingWindowReceiver creates a receiver.
func NewSlidingWindowReceiver(rcvWindow int) *SlidingWindowReceiver {
	return &SlidingWindowReceiver{rcvWin: rcvWindow, buffer: make(map[int]*Segment)}
}

// Receive processes an incoming segment.
// Returns (ackNum, outOfOrder) where ackNum is the cumulative ACK to send.
func (r *SlidingWindowReceiver) Receive(seg *Segment) (ackNum int, outOfOrder bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if seg.SeqNum == r.expected {
		r.expected++
		atomic.AddInt64(&r.Received, 1)
		// Deliver any buffered contiguous segments.
		for {
			if _, ok := r.buffer[r.expected]; ok {
				delete(r.buffer, r.expected)
				r.expected++
				atomic.AddInt64(&r.Received, 1)
			} else {
				break
			}
		}
		return r.expected - 1, false
	}

	// Out-of-order: buffer it.
	if seg.SeqNum > r.expected && len(r.buffer) < r.rcvWin {
		r.buffer[seg.SeqNum] = seg
		return r.expected - 1, true
	}
	return r.expected - 1, true
}

// Expected returns the next expected sequence number.
func (r *SlidingWindowReceiver) Expected() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.expected
}

// =============================================================================
// NET-003: ARQ Protocols
// =============================================================================

// ARQMetrics holds performance statistics for an ARQ protocol simulation.
type ARQMetrics struct {
	TotalFramesSent int
	Retransmissions int
	FramesDelivered int
	Efficiency      float64 // delivered / total sent
}

// StopAndWait simulates Stop-and-Wait ARQ over `frames` data frames
// with the given loss rate. Returns the transmission log and metrics.
func StopAndWait(frames int, lossRate float64, seed int64) ([]string, ARQMetrics) {
	rng := rand.New(rand.NewSource(seed))
	var log []string
	m := ARQMetrics{}

	for seq := 0; seq < frames; {
		m.TotalFramesSent++
		lost := rng.Float64() < lossRate
		if lost {
			m.Retransmissions++
			log = append(log, fmt.Sprintf("frame %d: LOST (retransmit)", seq))
			continue
		}
		log = append(log, fmt.Sprintf("frame %d: delivered", seq))
		m.FramesDelivered++
		seq++
	}
	if m.TotalFramesSent > 0 {
		m.Efficiency = float64(m.FramesDelivered) / float64(m.TotalFramesSent)
	}
	return log, m
}

// GoBackN simulates Go-Back-N ARQ with window size N.
// Returns the transmission log and metrics.
func GoBackN(frames, windowSize int, lossRate float64, seed int64) ([]string, ARQMetrics) {
	rng := rand.New(rand.NewSource(seed))
	var log []string
	m := ARQMetrics{}

	base := 0
	nextSeq := 0

	for base < frames {
		// Send up to windowSize frames.
		sent := 0
		for nextSeq < base+windowSize && nextSeq < frames {
			m.TotalFramesSent++
			lost := rng.Float64() < lossRate
			if lost {
				log = append(log, fmt.Sprintf("frame %d: LOST", nextSeq))
			} else {
				log = append(log, fmt.Sprintf("frame %d: sent", nextSeq))
			}
			nextSeq++
			sent++
		}
		_ = sent

		// Simulate ACK reception for frames from base.
		// The first lost frame triggers Go-Back-N retransmission.
		firstLost := -1
		for i := base; i < nextSeq; i++ {
			// Re-check loss for ACK (simplified: re-use same loss rate).
			if rng.Float64() < lossRate {
				firstLost = i
				break
			}
		}
		if firstLost == -1 {
			// All acked.
			m.FramesDelivered += nextSeq - base
			base = nextSeq
		} else {
			// Ack up to firstLost-1, then go-back-n from firstLost.
			if firstLost > base {
				m.FramesDelivered += firstLost - base
			}
			m.Retransmissions += nextSeq - firstLost
			log = append(log, fmt.Sprintf("GBN: retransmitting from frame %d", firstLost))
			nextSeq = firstLost
			base = firstLost
		}
	}
	if m.TotalFramesSent > 0 {
		m.Efficiency = float64(m.FramesDelivered) / float64(m.TotalFramesSent)
	}
	return log, m
}

// SelectiveRepeat simulates Selective Repeat ARQ.
// Returns the transmission log and metrics.
func SelectiveRepeat(frames, windowSize int, lossRate float64, seed int64) ([]string, ARQMetrics) {
	rng := rand.New(rand.NewSource(seed))
	var log []string
	m := ARQMetrics{}

	delivered := make([]bool, frames)
	acked := make([]bool, frames)
	base := 0

	for base < frames {
		// Send window.
		end := base + windowSize
		if end > frames {
			end = frames
		}
		for seq := base; seq < end; seq++ {
			if acked[seq] {
				continue
			}
			m.TotalFramesSent++
			if rng.Float64() < lossRate {
				log = append(log, fmt.Sprintf("frame %d: LOST", seq))
			} else {
				log = append(log, fmt.Sprintf("frame %d: delivered", seq))
				delivered[seq] = true
			}
		}

		// Process ACKs for delivered frames in window.
		for seq := base; seq < end; seq++ {
			if delivered[seq] && !acked[seq] {
				acked[seq] = true
				m.FramesDelivered++
			}
		}

		// Retransmit lost frames.
		for seq := base; seq < end; seq++ {
			if !delivered[seq] && !acked[seq] {
				m.TotalFramesSent++
				m.Retransmissions++
				log = append(log, fmt.Sprintf("frame %d: retransmit", seq))
				delivered[seq] = true
				acked[seq] = true
				m.FramesDelivered++
			}
		}

		// Advance base.
		for base < frames && acked[base] {
			base++
		}
	}
	if m.TotalFramesSent > 0 {
		m.Efficiency = float64(m.FramesDelivered) / float64(m.TotalFramesSent)
	}
	return log, m
}

// =============================================================================
// NET-004: Nagle's Algorithm
// =============================================================================

// NagleBuffer implements Nagle's algorithm for small segment coalescing.
// It accumulates data until either:
//   - A full MSS-sized segment is available, or
//   - No unacknowledged data is outstanding.
type NagleBuffer struct {
	mu          sync.Mutex
	mss         int    // maximum segment size
	buffer      []byte // pending data not yet sent
	outstanding int    // bytes sent but not yet ACKed
	disabled    bool   // TCP_NODELAY flag

	// Metrics
	SegmentsSent  int64
	BytesSent     int64
	CoalesceSaved int64 // bytes that would have been sent as tiny segments
}

// NewNagleBuffer creates a Nagle buffer with the given MSS.
func NewNagleBuffer(mss int) *NagleBuffer {
	return &NagleBuffer{mss: mss}
}

// SetNoDelay disables Nagle (TCP_NODELAY).
func (n *NagleBuffer) SetNoDelay(v bool) {
	n.mu.Lock()
	n.disabled = v
	n.mu.Unlock()
}

// Write adds data to the Nagle buffer and returns segments ready to send.
// Each returned byte slice represents one segment that should be transmitted.
func (n *NagleBuffer) Write(data []byte) [][]byte {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.buffer = append(n.buffer, data...)
	return n.flush()
}

// Ack acknowledges `bytes` bytes, potentially allowing buffered data to be sent.
// Returns any newly flushable segments.
func (n *NagleBuffer) Ack(bytes int) [][]byte {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.outstanding -= bytes
	if n.outstanding < 0 {
		n.outstanding = 0
	}
	return n.flush()
}

// flush returns segments to send based on current Nagle state.
// Must be called with mu held.
func (n *NagleBuffer) flush() [][]byte {
	var segments [][]byte
	for len(n.buffer) > 0 {
		canSend := false
		if n.disabled {
			canSend = true
		} else if len(n.buffer) >= n.mss {
			canSend = true // full MSS available
		} else if n.outstanding == 0 {
			canSend = true // no outstanding data
		}
		if !canSend {
			break
		}

		size := len(n.buffer)
		if size > n.mss {
			size = n.mss
		}
		seg := make([]byte, size)
		copy(seg, n.buffer[:size])
		n.buffer = n.buffer[size:]
		n.outstanding += size

		atomic.AddInt64(&n.SegmentsSent, 1)
		atomic.AddInt64(&n.BytesSent, int64(size))
		segments = append(segments, seg)
	}
	return segments
}

// Pending returns the number of bytes waiting in the buffer.
func (n *NagleBuffer) Pending() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.buffer)
}

// =============================================================================
// NET-005: Routing Table (Longest Prefix Match)
// =============================================================================

// Route represents a routing table entry.
type Route struct {
	Network  uint32 // network address
	Mask     uint32 // subnet mask
	NextHop  string // next hop address or interface
	PrefixLen int   // number of set bits in mask (for LPM)
}

// RoutingTable implements a simple routing table with longest-prefix match.
type RoutingTable struct {
	mu     sync.RWMutex
	routes []*Route
}

// NewRoutingTable creates an empty routing table.
func NewRoutingTable() *RoutingTable {
	return &RoutingTable{}
}

// AddRoute adds a route to the table. If an identical network/mask already
// exists, it is replaced.
func (rt *RoutingTable) AddRoute(network, mask uint32, nextHop string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	prefixLen := popcount(mask)
	// Replace existing.
	for _, r := range rt.routes {
		if r.Network == network && r.Mask == mask {
			r.NextHop = nextHop
			return
		}
	}
	rt.routes = append(rt.routes, &Route{
		Network:   network,
		Mask:      mask,
		NextHop:   nextHop,
		PrefixLen: prefixLen,
	})
	// Keep sorted by prefix length descending for LPM.
	sort.Slice(rt.routes, func(i, j int) bool {
		return rt.routes[i].PrefixLen > rt.routes[j].PrefixLen
	})
}

// DeleteRoute removes the route matching network/mask.
func (rt *RoutingTable) DeleteRoute(network, mask uint32) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for i, r := range rt.routes {
		if r.Network == network && r.Mask == mask {
			rt.routes = append(rt.routes[:i], rt.routes[i+1:]...)
			return true
		}
	}
	return false
}

// Route performs longest-prefix match for the given destination IP.
// Returns the matching route, or an error if no route found.
func (rt *RoutingTable) Route(destIP uint32) (*Route, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Routes are sorted by prefix length descending — first match wins.
	for _, r := range rt.routes {
		if destIP&r.Mask == r.Network {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no route to host %s", uint32ToIP(destIP))
}

// Len returns the number of routes in the table.
func (rt *RoutingTable) Len() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.routes)
}

// ParseIPv4 converts dotted-decimal string to uint32.
func ParseIPv4(s string) (uint32, error) {
	var a, b, c, d uint32
	n, err := fmt.Sscanf(s, "%d.%d.%d.%d", &a, &b, &c, &d)
	if err != nil || n != 4 {
		return 0, fmt.Errorf("invalid IP address: %s", s)
	}
	if a > 255 || b > 255 || c > 255 || d > 255 {
		return 0, fmt.Errorf("octet out of range in: %s", s)
	}
	return (a << 24) | (b << 16) | (c << 8) | d, nil
}

// MaskFromPrefix returns a subnet mask for the given prefix length.
func MaskFromPrefix(bits int) uint32 {
	if bits == 0 {
		return 0
	}
	if bits >= 32 {
		return 0xFFFFFFFF
	}
	return ^uint32((1 << (32 - bits)) - 1)
}

func uint32ToIP(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", ip>>24, (ip>>16)&0xFF, (ip>>8)&0xFF, ip&0xFF)
}

func popcount(x uint32) int {
	n := 0
	for x != 0 {
		n += int(x & 1)
		x >>= 1
	}
	return n
}

// ErrInvalidTransition signals an invalid TCP state transition.
var ErrInvalidTransition = errors.New("invalid TCP state transition")
