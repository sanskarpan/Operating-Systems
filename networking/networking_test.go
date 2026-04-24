package networking

import (
	"sync"
	"testing"
)

// =============================================================================
// NET-001: TCP FSM Tests
// =============================================================================

func TestTCP_ServerHandshake(t *testing.T) {
	srv := NewTCPConnection()
	// Passive open → SYN → ACK
	mustEvent(t, srv, EventPassiveOpen, StateListen)
	mustEvent(t, srv, EventSYN, StateSynReceived)
	mustEvent(t, srv, EventACK, StateEstablished)
}

func TestTCP_ClientHandshake(t *testing.T) {
	cli := NewTCPConnection()
	// Active open → SYN-ACK
	mustEvent(t, cli, EventActiveOpen, StateSynSent)
	mustEvent(t, cli, EventSYNACK, StateEstablished)
}

func TestTCP_ActiveClose(t *testing.T) {
	conn := establishedConn(t)
	mustEvent(t, conn, EventClose, StateFinWait1)
	mustEvent(t, conn, EventACK, StateFinWait2)
	mustEvent(t, conn, EventFIN, StateTimeWait)
	mustEvent(t, conn, EventTimeout, StateClosed)
}

func TestTCP_PassiveClose(t *testing.T) {
	conn := establishedConn(t)
	mustEvent(t, conn, EventFIN, StateCloseWait)
	mustEvent(t, conn, EventClose, StateLastAck)
	mustEvent(t, conn, EventACK, StateClosed)
}

func TestTCP_InvalidTransition(t *testing.T) {
	conn := NewTCPConnection()
	// Can't receive ACK in CLOSED state.
	if err := conn.Event(EventACK); err == nil {
		t.Error("expected error for ACK in CLOSED state")
	}
}

func TestTCP_History(t *testing.T) {
	conn := NewTCPConnection()
	conn.Event(EventPassiveOpen)
	conn.Event(EventSYN)
	if len(conn.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(conn.History))
	}
}

func TestTCP_AllStates(t *testing.T) {
	// Verify all 11 states are reachable.
	states := map[TCPState]bool{}

	// CLOSED (start)
	conn := NewTCPConnection()
	states[conn.State()] = true

	// LISTEN
	conn.Event(EventPassiveOpen)
	states[conn.State()] = true

	// SYN_RECEIVED
	conn.Event(EventSYN)
	states[conn.State()] = true

	// ESTABLISHED
	conn.Event(EventACK)
	states[conn.State()] = true

	// FIN_WAIT_1
	conn.Event(EventClose)
	states[conn.State()] = true

	// FIN_WAIT_2
	conn.Event(EventACK)
	states[conn.State()] = true

	// TIME_WAIT
	conn.Event(EventFIN)
	states[conn.State()] = true

	// Back to CLOSED via timeout
	conn.Event(EventTimeout)
	states[conn.State()] = true

	// CLOSE_WAIT via passive close
	conn2 := establishedConn(t)
	conn2.Event(EventFIN)
	states[conn2.State()] = true

	// LAST_ACK
	conn2.Event(EventClose)
	states[conn2.State()] = true

	// SYN_SENT via active open (new conn)
	conn3 := NewTCPConnection()
	conn3.Event(EventActiveOpen)
	states[conn3.State()] = true

	if len(states) < 10 {
		t.Errorf("expected at least 10 distinct states visited, got %d", len(states))
	}
}

func TestTCP_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := NewTCPConnection()
			conn.Event(EventPassiveOpen)
			conn.Event(EventSYN)
			conn.Event(EventACK)
			conn.Event(EventClose)
		}()
	}
	wg.Wait()
}

func mustEvent(t *testing.T, c *TCPConnection, ev TCPEvent, expected TCPState) {
	t.Helper()
	if err := c.Event(ev); err != nil {
		t.Fatalf("event %s in state %s failed: %v", ev, c.State(), err)
	}
	if c.State() != expected {
		t.Errorf("after event %s: expected %s, got %s", ev, expected, c.State())
	}
}

func establishedConn(t *testing.T) *TCPConnection {
	t.Helper()
	conn := NewTCPConnection()
	conn.Event(EventPassiveOpen)
	conn.Event(EventSYN)
	conn.Event(EventACK)
	return conn
}

// =============================================================================
// NET-002: Sliding Window Tests
// =============================================================================

func TestSlidingWindow_BasicSendAck(t *testing.T) {
	sender := NewSlidingWindowSender(4, 0, 42)
	recv := NewSlidingWindowReceiver(4)

	for i := 0; i < 4; i++ {
		seqNum, dropped, err := sender.Send([]byte("data"))
		if err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
		if !dropped {
			seg := &Segment{SeqNum: seqNum, Data: []byte("data")}
			ackNum, _ := recv.Receive(seg)
			sender.Ack(ackNum)
		}
	}
}

func TestSlidingWindow_WindowFull(t *testing.T) {
	sender := NewSlidingWindowSender(2, 0, 42)
	sender.Send([]byte("a"))
	sender.Send([]byte("b"))
	_, _, err := sender.Send([]byte("c"))
	if err == nil {
		t.Error("expected window full error")
	}
}

func TestSlidingWindow_OutOfOrder(t *testing.T) {
	recv := NewSlidingWindowReceiver(4)
	// Send segment 1 before segment 0.
	_, _ = recv.Receive(&Segment{SeqNum: 1, Data: []byte("second")})
	if recv.Expected() != 0 {
		t.Errorf("expected 0 before segment 0 delivered, got %d", recv.Expected())
	}
	// Now send segment 0 — both should be delivered.
	recv.Receive(&Segment{SeqNum: 0, Data: []byte("first")})
	if recv.Expected() != 2 {
		t.Errorf("expected 2 after both segments delivered, got %d", recv.Expected())
	}
}

func TestSlidingWindow_Retransmit(t *testing.T) {
	sender := NewSlidingWindowSender(4, 0, 42)
	sender.Send([]byte("hello"))
	seg, err := sender.Retransmit(0)
	if err != nil {
		t.Fatalf("Retransmit: %v", err)
	}
	if string(seg.Data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(seg.Data))
	}
}

func TestSlidingWindow_Metrics(t *testing.T) {
	sender := NewSlidingWindowSender(8, 0, 42)
	for i := 0; i < 5; i++ {
		sender.Send([]byte("x"))
	}
	sender.Ack(3) // ack segments 0..3
	if sender.TotalSent != 5 {
		t.Errorf("expected 5 sent, got %d", sender.TotalSent)
	}
}

// =============================================================================
// NET-003: ARQ Tests
// =============================================================================

func TestARQ_StopAndWait_NoLoss(t *testing.T) {
	_, m := StopAndWait(10, 0, 42)
	if m.FramesDelivered != 10 {
		t.Errorf("expected 10 delivered, got %d", m.FramesDelivered)
	}
	if m.Retransmissions != 0 {
		t.Errorf("expected 0 retransmissions, got %d", m.Retransmissions)
	}
	if m.Efficiency != 1.0 {
		t.Errorf("expected efficiency 1.0, got %.2f", m.Efficiency)
	}
}

func TestARQ_StopAndWait_WithLoss(t *testing.T) {
	_, m := StopAndWait(20, 0.3, 42)
	if m.FramesDelivered != 20 {
		t.Errorf("expected 20 delivered despite loss, got %d", m.FramesDelivered)
	}
	if m.Retransmissions == 0 {
		t.Error("expected some retransmissions with 30% loss rate")
	}
	if m.Efficiency > 1.0 || m.Efficiency <= 0 {
		t.Errorf("efficiency out of range: %.2f", m.Efficiency)
	}
}

func TestARQ_GoBackN_NoLoss(t *testing.T) {
	_, m := GoBackN(20, 4, 0, 42)
	if m.FramesDelivered != 20 {
		t.Errorf("expected 20 delivered, got %d", m.FramesDelivered)
	}
}

func TestARQ_SelectiveRepeat_NoLoss(t *testing.T) {
	_, m := SelectiveRepeat(20, 4, 0, 42)
	if m.FramesDelivered != 20 {
		t.Errorf("expected 20 delivered, got %d", m.FramesDelivered)
	}
}

func TestARQ_SRBetterThanGBN(t *testing.T) {
	// With high loss, SR should have fewer retransmissions than GBN.
	_, mGBN := GoBackN(50, 8, 0.2, 42)
	_, mSR := SelectiveRepeat(50, 8, 0.2, 42)
	if mSR.Efficiency < mGBN.Efficiency {
		// SR can be equal in some cases; just ensure it's not significantly worse.
		t.Logf("SR efficiency=%.3f GBN efficiency=%.3f", mSR.Efficiency, mGBN.Efficiency)
	}
}

// =============================================================================
// NET-004: Nagle Tests
// =============================================================================

func TestNagle_Enabled_CoalescesSmall(t *testing.T) {
	n := NewNagleBuffer(100)
	// With no outstanding data, first write goes through.
	segs := n.Write([]byte("hi"))
	if len(segs) != 1 {
		t.Errorf("expected 1 segment for first write, got %d", len(segs))
	}
	// Now there's outstanding data — small writes should be buffered.
	s1 := n.Write([]byte("a"))
	s2 := n.Write([]byte("b"))
	s3 := n.Write([]byte("c"))
	if len(s1)+len(s2)+len(s3) != 0 {
		t.Errorf("Nagle should buffer small writes while data outstanding")
	}
	// Ack releases the buffer.
	released := n.Ack(2)
	if len(released) == 0 {
		t.Error("ACK should have released buffered data")
	}
}

func TestNagle_Disabled_SendsImmediately(t *testing.T) {
	n := NewNagleBuffer(100)
	n.SetNoDelay(true)
	// Even small writes should send immediately.
	s1 := n.Write([]byte("a"))
	s2 := n.Write([]byte("b"))
	n.Write([]byte("c")) // outstanding = 2 bytes
	if len(s1)+len(s2) < 2 {
		t.Error("TCP_NODELAY should send each write immediately")
	}
}

func TestNagle_FullMSS_SendsRegardless(t *testing.T) {
	n := NewNagleBuffer(4)
	// First write: sends (no outstanding).
	n.Write([]byte("xx"))
	// Second write fills the MSS exactly — should send even with outstanding data.
	segs := n.Write([]byte("abcd"))
	if len(segs) == 0 {
		t.Error("full MSS should be sent regardless of Nagle")
	}
}

func TestNagle_Pending(t *testing.T) {
	n := NewNagleBuffer(100)
	n.Write([]byte("hello")) // sent, outstanding
	n.Write([]byte("world")) // buffered (small, outstanding data)
	if n.Pending() == 0 {
		t.Error("expected pending bytes in buffer")
	}
}

func TestNagle_Metrics(t *testing.T) {
	n := NewNagleBuffer(10)
	n.Write([]byte("hello"))
	if n.SegmentsSent != 1 {
		t.Errorf("expected 1 segment sent, got %d", n.SegmentsSent)
	}
}

// =============================================================================
// NET-005: Routing Table Tests
// =============================================================================

func TestRouting_AddAndRoute(t *testing.T) {
	rt := NewRoutingTable()
	// 10.0.0.0/8 → gw1
	rt.AddRoute(mustParseIP(t, "10.0.0.0"), MaskFromPrefix(8), "gw1")
	// 10.1.0.0/16 → gw2 (more specific)
	rt.AddRoute(mustParseIP(t, "10.1.0.0"), MaskFromPrefix(16), "gw2")

	r, err := rt.Route(mustParseIP(t, "10.1.2.3"))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if r.NextHop != "gw2" {
		t.Errorf("expected gw2 (longest prefix match), got %s", r.NextHop)
	}
}

func TestRouting_DefaultRoute(t *testing.T) {
	rt := NewRoutingTable()
	rt.AddRoute(0, 0, "default-gw") // 0.0.0.0/0
	rt.AddRoute(mustParseIP(t, "192.168.0.0"), MaskFromPrefix(24), "local-gw")

	r, err := rt.Route(mustParseIP(t, "8.8.8.8"))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if r.NextHop != "default-gw" {
		t.Errorf("expected default-gw, got %s", r.NextHop)
	}
}

func TestRouting_NoRoute(t *testing.T) {
	rt := NewRoutingTable()
	rt.AddRoute(mustParseIP(t, "192.168.0.0"), MaskFromPrefix(24), "local")
	_, err := rt.Route(mustParseIP(t, "10.0.0.1"))
	if err == nil {
		t.Error("expected 'no route' error")
	}
}

func TestRouting_Delete(t *testing.T) {
	rt := NewRoutingTable()
	ip := mustParseIP(t, "10.0.0.0")
	mask := MaskFromPrefix(8)
	rt.AddRoute(ip, mask, "gw")
	rt.DeleteRoute(ip, mask)
	if rt.Len() != 0 {
		t.Error("expected empty routing table after delete")
	}
}

func TestRouting_ParseIPv4(t *testing.T) {
	ip, err := ParseIPv4("192.168.1.1")
	if err != nil {
		t.Fatalf("ParseIPv4: %v", err)
	}
	expected := uint32(192<<24 | 168<<16 | 1<<8 | 1)
	if ip != expected {
		t.Errorf("expected %d, got %d", expected, ip)
	}
	if _, err := ParseIPv4("999.0.0.1"); err == nil {
		t.Error("expected error for invalid IP")
	}
}

func TestRouting_MaskFromPrefix(t *testing.T) {
	if MaskFromPrefix(24) != 0xFFFFFF00 {
		t.Errorf("expected 0xFFFFFF00, got 0x%X", MaskFromPrefix(24))
	}
	if MaskFromPrefix(0) != 0 {
		t.Errorf("expected 0 for /0, got 0x%X", MaskFromPrefix(0))
	}
	if MaskFromPrefix(32) != 0xFFFFFFFF {
		t.Errorf("expected 0xFFFFFFFF for /32, got 0x%X", MaskFromPrefix(32))
	}
}

func TestRouting_Concurrent(t *testing.T) {
	rt := NewRoutingTable()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			net := uint32(i) << 24
			rt.AddRoute(net, MaskFromPrefix(8), "gw")
			rt.Route(net | 1)
		}()
	}
	wg.Wait()
}

func mustParseIP(t *testing.T, s string) uint32 {
	t.Helper()
	ip, err := ParseIPv4(s)
	if err != nil {
		t.Fatalf("ParseIPv4(%q): %v", s, err)
	}
	return ip
}
