package ipc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMessageQueue(t *testing.T) {
	mq := NewMessageQueue(10)

	msg := Message{
		From: 1,
		To:   2,
		Type: "test",
		Data: []byte("hello"),
	}

	err := mq.Send(msg)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	received, err := mq.Receive()
	if err != nil {
		t.Errorf("Receive failed: %v", err)
	}

	if received.From != msg.From || string(received.Data) != string(msg.Data) {
		t.Error("Received message doesn't match sent message")
	}
}

func TestMessageQueueBlocking(t *testing.T) {
	mq := NewMessageQueue(2)

	// Fill queue
	mq.Send(Message{From: 1, Data: []byte("1")})
	mq.Send(Message{From: 1, Data: []byte("2")})

	// Try to send when full (should block)
	done := make(chan bool)
	go func() {
		mq.Send(Message{From: 1, Data: []byte("3")})
		done <- true
	}()

	time.Sleep(100 * time.Millisecond)

	// Receive to unblock
	mq.Receive()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("Send did not unblock")
	}
}

func TestMailboxSystem(t *testing.T) {
	ms := NewMailboxSystem()

	// Create mailbox
	mbID := ms.CreateMailbox(1, 5)

	// Send message
	msg := Message{From: 2, Data: []byte("test")}
	err := ms.Send(mbID, msg)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	// Receive message
	received, err := ms.Receive(mbID)
	if err != nil {
		t.Errorf("Receive failed: %v", err)
	}

	if string(received.Data) != "test" {
		t.Errorf("Expected 'test', got '%s'", string(received.Data))
	}

	// Delete mailbox
	err = ms.DeleteMailbox(mbID)
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}
}

func TestPipe(t *testing.T) {
	pipe := NewPipe(100)

	data := []byte("hello world")
	written, err := pipe.Write(data)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if written != len(data) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(data), written)
	}

	read, err := pipe.Read(5)
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}

	if string(read) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(read))
	}
}

func TestPipeClose(t *testing.T) {
	pipe := NewPipe(10)

	pipe.Close()

	_, err := pipe.Write([]byte("test"))
	if err == nil {
		t.Error("Write to closed pipe should fail")
	}
}

func TestSharedMemory(t *testing.T) {
	smm := NewSharedMemoryManager()

	// Create segment
	segID := smm.Create(100)

	// Attach to segment
	seg, err := smm.Attach(segID)
	if err != nil {
		t.Errorf("Attach failed: %v", err)
	}

	// Write to shared memory
	data := []byte("shared data")
	err = seg.Write(0, data)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Read from shared memory
	read, err := seg.Read(0, len(data))
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}

	if string(read) != string(data) {
		t.Errorf("Expected '%s', got '%s'", string(data), string(read))
	}

	// Destroy segment
	err = smm.Destroy(segID)
	if err != nil {
		t.Errorf("Destroy failed: %v", err)
	}
}

func TestSharedMemoryConcurrent(t *testing.T) {
	smm := NewSharedMemoryManager()
	segID := smm.Create(1000)
	seg, _ := smm.Attach(segID)

	var wg sync.WaitGroup
	numWriters := 5

	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			data := []byte{byte(id)}
			seg.Write(id*10, data)
		}(i)
	}

	wg.Wait()

	// Verify writes
	for i := 0; i < numWriters; i++ {
		data, _ := seg.Read(i*10, 1)
		if data[0] != byte(i) {
			t.Errorf("Expected %d at offset %d, got %d", i, i*10, data[0])
		}
	}
}

func TestSignalHandler(t *testing.T) {
	pst := NewProcessSignalTable()

	// Use atomic to avoid a data race between the handler goroutine and the test.
	var received int32
	handler := func(sig SignalType) {
		atomic.StoreInt32(&received, 1)
	}

	pst.RegisterHandler(SIGTERM, handler)
	pst.SendSignal(SIGTERM)

	// Give time for signal delivery
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&received) == 0 {
		t.Error("Signal handler not called")
	}
}

func TestSocketManager(t *testing.T) {
	sm := NewSocketManager()

	// Create sockets
	sock1 := sm.CreateSocket(Stream)
	sock2 := sm.CreateSocket(Stream)

	// Bind to ports
	err := sm.Bind(sock1, 8080)
	if err != nil {
		t.Errorf("Bind failed: %v", err)
	}

	err = sm.Bind(sock2, 8081)
	if err != nil {
		t.Errorf("Bind failed: %v", err)
	}

	// Try to bind same port again (should fail)
	sock3 := sm.CreateSocket(Stream)
	err = sm.Bind(sock3, 8080)
	if err == nil {
		t.Error("Should not be able to bind to already used port")
	}

	// Send data
	err = sm.SendTo(sock1, 8081, []byte("hello"))
	if err != nil {
		t.Errorf("SendTo failed: %v", err)
	}

	// Receive data (in separate goroutine to avoid blocking)
	done := make(chan Message)
	go func() {
		msg, _ := sm.ReceiveFrom(sock2)
		done <- msg
	}()

	select {
	case msg := <-done:
		if string(msg.Data) != "hello" {
			t.Errorf("Expected 'hello', got '%s'", string(msg.Data))
		}
	case <-time.After(time.Second):
		t.Error("Receive timed out")
	}

	// Close sockets
	sm.Close(sock1)
	sm.Close(sock2)
}

func TestRPCServer(t *testing.T) {
	server := NewRPCServer()

	// Register method
	server.Register("add", func(params []interface{}) (interface{}, error) {
		a := params[0].(int)
		b := params[1].(int)
		return a + b, nil
	})

	// Start server
	server.Serve()

	// Make RPC call
	result, err := server.Call("add", []interface{}{5, 3})
	if err != nil {
		t.Errorf("RPC call failed: %v", err)
	}

	if result.(int) != 8 {
		t.Errorf("Expected 8, got %v", result)
	}

	server.Stop()
}

func TestRPCServerConcurrent(t *testing.T) {
	server := NewRPCServer()

	server.Register("double", func(params []interface{}) (interface{}, error) {
		n := params[0].(int)
		return n * 2, nil
	})

	server.Serve()
	defer server.Stop()

	const numCallers = 20
	results := make([]int, numCallers)
	errs := make([]error, numCallers)

	var wg sync.WaitGroup
	wg.Add(numCallers)

	for i := 0; i < numCallers; i++ {
		i := i
		go func() {
			defer wg.Done()
			res, err := server.Call("double", []interface{}{i})
			errs[i] = err
			if err == nil {
				results[i] = res.(int)
			}
		}()
	}

	wg.Wait()

	for i := 0; i < numCallers; i++ {
		if errs[i] != nil {
			t.Errorf("Caller %d got error: %v", i, errs[i])
			continue
		}
		if results[i] != i*2 {
			t.Errorf("Caller %d: expected %d, got %d", i, i*2, results[i])
		}
	}
}

func TestMessageQueueTrySend(t *testing.T) {
	mq := NewMessageQueue(2)

	msg1 := Message{Data: []byte("1")}
	msg2 := Message{Data: []byte("2")}
	msg3 := Message{Data: []byte("3")}

	// First two should succeed
	if !mq.TrySend(msg1) {
		t.Error("TrySend should succeed")
	}

	if !mq.TrySend(msg2) {
		t.Error("TrySend should succeed")
	}

	// Third should fail (queue full)
	if mq.TrySend(msg3) {
		t.Error("TrySend should fail on full queue")
	}
}

func TestMessageQueueTryReceive(t *testing.T) {
	mq := NewMessageQueue(10)

	// Try receive from empty queue
	_, ok := mq.TryReceive()
	if ok {
		t.Error("TryReceive should fail on empty queue")
	}

	// Add message
	mq.Send(Message{Data: []byte("test")})

	// Should succeed now
	msg, ok := mq.TryReceive()
	if !ok {
		t.Error("TryReceive should succeed")
	}

	if string(msg.Data) != "test" {
		t.Errorf("Expected 'test', got '%s'", string(msg.Data))
	}
}
