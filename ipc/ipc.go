/*
Inter-Process Communication (IPC)
==================================

Implementation of IPC mechanisms for process coordination.

Applications:
- Microservices communication
- Distributed systems
- Multi-process applications
- Operating system services
*/

package ipc

import (
	"errors"
	"sync"
	"time"
)

// =============================================================================
// Message Passing
// =============================================================================

// Message represents an IPC message
type Message struct {
	From    int
	To      int
	Type    string
	Data    []byte
	Sent    time.Time
}

// MessageQueue implements a message queue for IPC
type MessageQueue struct {
	queue    []Message
	capacity int
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
}

// NewMessageQueue creates a new message queue
func NewMessageQueue(capacity int) *MessageQueue {
	mq := &MessageQueue{
		queue:    make([]Message, 0, capacity),
		capacity: capacity,
	}
	mq.notEmpty = sync.NewCond(&mq.mu)
	mq.notFull = sync.NewCond(&mq.mu)
	return mq
}

// Send sends a message to the queue
func (mq *MessageQueue) Send(msg Message) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	for len(mq.queue) >= mq.capacity {
		mq.notFull.Wait()
	}

	msg.Sent = time.Now()
	mq.queue = append(mq.queue, msg)
	mq.notEmpty.Signal()

	return nil
}

// Receive receives a message from the queue
func (mq *MessageQueue) Receive() (Message, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	for len(mq.queue) == 0 {
		mq.notEmpty.Wait()
	}

	msg := mq.queue[0]
	mq.queue = mq.queue[1:]
	mq.notFull.Signal()

	return msg, nil
}

// TrySend attempts to send without blocking
func (mq *MessageQueue) TrySend(msg Message) bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.queue) >= mq.capacity {
		return false
	}

	msg.Sent = time.Now()
	mq.queue = append(mq.queue, msg)
	mq.notEmpty.Signal()

	return true
}

// TryReceive attempts to receive without blocking
func (mq *MessageQueue) TryReceive() (Message, bool) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.queue) == 0 {
		return Message{}, false
	}

	msg := mq.queue[0]
	mq.queue = mq.queue[1:]
	mq.notFull.Signal()

	return msg, true
}

// Size returns current queue size
func (mq *MessageQueue) Size() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.queue)
}

// =============================================================================
// Mailbox System
// =============================================================================

// Mailbox represents a mailbox for process communication
type Mailbox struct {
	ID       int
	Owner    int
	Messages []Message
	Capacity int
	mu       sync.Mutex
}

// MailboxSystem manages multiple mailboxes
type MailboxSystem struct {
	mailboxes map[int]*Mailbox
	nextID    int
	mu        sync.RWMutex
}

// NewMailboxSystem creates a new mailbox system
func NewMailboxSystem() *MailboxSystem {
	return &MailboxSystem{
		mailboxes: make(map[int]*Mailbox),
		nextID:    1,
	}
}

// CreateMailbox creates a new mailbox
func (ms *MailboxSystem) CreateMailbox(owner int, capacity int) int {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	mb := &Mailbox{
		ID:       ms.nextID,
		Owner:    owner,
		Messages: make([]Message, 0, capacity),
		Capacity: capacity,
	}

	ms.mailboxes[ms.nextID] = mb
	id := ms.nextID
	ms.nextID++

	return id
}

// Send sends a message to a mailbox
func (ms *MailboxSystem) Send(mailboxID int, msg Message) error {
	ms.mu.RLock()
	mb, exists := ms.mailboxes[mailboxID]
	ms.mu.RUnlock()

	if !exists {
		return errors.New("mailbox not found")
	}

	mb.mu.Lock()
	defer mb.mu.Unlock()

	if len(mb.Messages) >= mb.Capacity {
		return errors.New("mailbox full")
	}

	msg.To = mailboxID
	msg.Sent = time.Now()
	mb.Messages = append(mb.Messages, msg)

	return nil
}

// Receive receives a message from a mailbox
func (ms *MailboxSystem) Receive(mailboxID int) (Message, error) {
	ms.mu.RLock()
	mb, exists := ms.mailboxes[mailboxID]
	ms.mu.RUnlock()

	if !exists {
		return Message{}, errors.New("mailbox not found")
	}

	mb.mu.Lock()
	defer mb.mu.Unlock()

	if len(mb.Messages) == 0 {
		return Message{}, errors.New("mailbox empty")
	}

	msg := mb.Messages[0]
	mb.Messages = mb.Messages[1:]

	return msg, nil
}

// DeleteMailbox deletes a mailbox
func (ms *MailboxSystem) DeleteMailbox(mailboxID int) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.mailboxes[mailboxID]; !exists {
		return errors.New("mailbox not found")
	}

	delete(ms.mailboxes, mailboxID)
	return nil
}

// =============================================================================
// Pipe (UNIX-style pipe)
// =============================================================================

// Pipe implements a unidirectional pipe
type Pipe struct {
	buffer   []byte
	capacity int
	size     int
	readPos  int
	writePos int
	closed   bool
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
}

// NewPipe creates a new pipe
func NewPipe(capacity int) *Pipe {
	p := &Pipe{
		buffer:   make([]byte, capacity),
		capacity: capacity,
		size:     0,
		readPos:  0,
		writePos: 0,
		closed:   false,
	}
	p.notEmpty = sync.NewCond(&p.mu)
	p.notFull = sync.NewCond(&p.mu)
	return p
}

// Write writes data to pipe
func (p *Pipe) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, errors.New("pipe closed")
	}

	written := 0
	for written < len(data) {
		for p.size >= p.capacity {
			p.notFull.Wait()
			if p.closed {
				return written, errors.New("pipe closed")
			}
		}

		toWrite := len(data) - written
		available := p.capacity - p.size
		if toWrite > available {
			toWrite = available
		}

		for i := 0; i < toWrite; i++ {
			p.buffer[p.writePos] = data[written+i]
			p.writePos = (p.writePos + 1) % p.capacity
		}

		p.size += toWrite
		written += toWrite
		p.notEmpty.Signal()
	}

	return written, nil
}

// Read reads data from pipe
func (p *Pipe) Read(size int) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for p.size == 0 {
		if p.closed {
			return nil, errors.New("pipe closed")
		}
		p.notEmpty.Wait()
	}

	toRead := size
	if toRead > p.size {
		toRead = p.size
	}

	data := make([]byte, toRead)
	for i := 0; i < toRead; i++ {
		data[i] = p.buffer[p.readPos]
		p.readPos = (p.readPos + 1) % p.capacity
	}

	p.size -= toRead
	p.notFull.Signal()

	return data, nil
}

// Close closes the pipe
func (p *Pipe) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	p.notEmpty.Broadcast()
	p.notFull.Broadcast()
}

// =============================================================================
// Shared Memory
// =============================================================================

// SharedMemory represents a shared memory segment
type SharedMemory struct {
	ID      int
	Size    int
	Data    []byte
	Readers int
	Writers int
	mu      sync.RWMutex
}

// SharedMemoryManager manages shared memory segments
type SharedMemoryManager struct {
	segments map[int]*SharedMemory
	nextID   int
	mu       sync.Mutex
}

// NewSharedMemoryManager creates a shared memory manager
func NewSharedMemoryManager() *SharedMemoryManager {
	return &SharedMemoryManager{
		segments: make(map[int]*SharedMemory),
		nextID:   1,
	}
}

// Create creates a new shared memory segment
func (smm *SharedMemoryManager) Create(size int) int {
	smm.mu.Lock()
	defer smm.mu.Unlock()

	seg := &SharedMemory{
		ID:   smm.nextID,
		Size: size,
		Data: make([]byte, size),
	}

	smm.segments[smm.nextID] = seg
	id := smm.nextID
	smm.nextID++

	return id
}

// Attach attaches to a shared memory segment (for reading/writing)
func (smm *SharedMemoryManager) Attach(segmentID int) (*SharedMemory, error) {
	smm.mu.Lock()
	defer smm.mu.Unlock()

	seg, exists := smm.segments[segmentID]
	if !exists {
		return nil, errors.New("segment not found")
	}

	return seg, nil
}

// Read reads from shared memory
func (sm *SharedMemory) Read(offset, size int) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if offset+size > sm.Size {
		return nil, errors.New("read beyond segment size")
	}

	data := make([]byte, size)
	copy(data, sm.Data[offset:offset+size])

	return data, nil
}

// Write writes to shared memory
func (sm *SharedMemory) Write(offset int, data []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if offset+len(data) > sm.Size {
		return errors.New("write beyond segment size")
	}

	copy(sm.Data[offset:], data)

	return nil
}

// Detach detaches from shared memory segment.
// This simulation does not track reference counts; Detach is intentionally a
// no-op here.  In a real OS implementation it would decrement the segment's
// reference count and free the segment when it reaches zero.
func (smm *SharedMemoryManager) Detach(segmentID int) error {
	smm.mu.Lock()
	defer smm.mu.Unlock()
	if _, exists := smm.segments[segmentID]; !exists {
		return errors.New("segment not found")
	}
	return nil
}

// Destroy destroys a shared memory segment
func (smm *SharedMemoryManager) Destroy(segmentID int) error {
	smm.mu.Lock()
	defer smm.mu.Unlock()

	if _, exists := smm.segments[segmentID]; !exists {
		return errors.New("segment not found")
	}

	delete(smm.segments, segmentID)
	return nil
}

// =============================================================================
// Signal
// =============================================================================

// SignalType represents different signal types
type SignalType int

const (
	SIGTERM SignalType = iota // Termination signal
	SIGINT                    // Interrupt signal
	SIGUSR1                   // User-defined signal 1
	SIGUSR2                   // User-defined signal 2
	SIGCONT                   // Continue signal
	SIGSTOP                   // Stop signal
)

// SignalHandler represents a signal handler function
type SignalHandler func(SignalType)

// ProcessSignalTable manages signals for a process
type ProcessSignalTable struct {
	handlers map[SignalType]SignalHandler
	pending  []SignalType
	mu       sync.Mutex
}

// NewProcessSignalTable creates a new signal table
func NewProcessSignalTable() *ProcessSignalTable {
	return &ProcessSignalTable{
		handlers: make(map[SignalType]SignalHandler),
		pending:  make([]SignalType, 0),
	}
}

// RegisterHandler registers a signal handler
func (pst *ProcessSignalTable) RegisterHandler(sig SignalType, handler SignalHandler) {
	pst.mu.Lock()
	defer pst.mu.Unlock()

	pst.handlers[sig] = handler
}

// SendSignal sends a signal to the process
func (pst *ProcessSignalTable) SendSignal(sig SignalType) {
	pst.mu.Lock()
	pst.pending = append(pst.pending, sig)
	pst.mu.Unlock()

	// Deliver signal
	go pst.deliverSignal(sig)
}

// deliverSignal delivers a pending signal and removes it from the pending list.
func (pst *ProcessSignalTable) deliverSignal(sig SignalType) {
	pst.mu.Lock()
	handler, exists := pst.handlers[sig]
	// Remove the first occurrence of sig from pending to prevent unbounded growth.
	for i, s := range pst.pending {
		if s == sig {
			pst.pending = append(pst.pending[:i], pst.pending[i+1:]...)
			break
		}
	}
	pst.mu.Unlock()

	if exists && handler != nil {
		handler(sig)
	}
}

// =============================================================================
// Socket (Simplified simulation)
// =============================================================================

// SocketType represents socket type
type SocketType int

const (
	Stream SocketType = iota // TCP-like
	Datagram                 // UDP-like
)

// Socket represents a communication socket
type Socket struct {
	ID       int
	Type     SocketType
	Bound    bool
	Port     int
	Buffer   []Message
	mu       sync.Mutex
	notEmpty *sync.Cond
}

// SocketManager manages sockets
type SocketManager struct {
	sockets map[int]*Socket
	ports   map[int]*Socket // Port -> Socket mapping
	nextID  int
	mu      sync.Mutex
}

// NewSocketManager creates a socket manager
func NewSocketManager() *SocketManager {
	return &SocketManager{
		sockets: make(map[int]*Socket),
		ports:   make(map[int]*Socket),
		nextID:  1,
	}
}

// CreateSocket creates a new socket
func (sm *SocketManager) CreateSocket(sockType SocketType) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sock := &Socket{
		ID:     sm.nextID,
		Type:   sockType,
		Bound:  false,
		Buffer: make([]Message, 0),
	}
	sock.notEmpty = sync.NewCond(&sock.mu)

	sm.sockets[sm.nextID] = sock
	id := sm.nextID
	sm.nextID++

	return id
}

// Bind binds socket to a port
func (sm *SocketManager) Bind(socketID, port int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sock, exists := sm.sockets[socketID]
	if !exists {
		return errors.New("socket not found")
	}

	if _, portTaken := sm.ports[port]; portTaken {
		return errors.New("port already in use")
	}

	sock.Bound = true
	sock.Port = port
	sm.ports[port] = sock

	return nil
}

// SendTo sends data to a port
func (sm *SocketManager) SendTo(socketID, destPort int, data []byte) error {
	sm.mu.Lock()
	destSock, exists := sm.ports[destPort]
	sm.mu.Unlock()

	if !exists {
		return errors.New("destination port not bound")
	}

	msg := Message{
		From: socketID,
		Data: data,
		Sent: time.Now(),
	}

	destSock.mu.Lock()
	destSock.Buffer = append(destSock.Buffer, msg)
	destSock.mu.Unlock()
	destSock.notEmpty.Signal()

	return nil
}

// ReceiveFrom receives data from socket
func (sm *SocketManager) ReceiveFrom(socketID int) (Message, error) {
	sm.mu.Lock()
	sock, exists := sm.sockets[socketID]
	sm.mu.Unlock()

	if !exists {
		return Message{}, errors.New("socket not found")
	}

	sock.mu.Lock()
	defer sock.mu.Unlock()

	for len(sock.Buffer) == 0 {
		sock.notEmpty.Wait()
	}

	msg := sock.Buffer[0]
	sock.Buffer = sock.Buffer[1:]

	return msg, nil
}

// Close closes a socket
func (sm *SocketManager) Close(socketID int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sock, exists := sm.sockets[socketID]
	if !exists {
		return errors.New("socket not found")
	}

	if sock.Bound {
		delete(sm.ports, sock.Port)
	}

	delete(sm.sockets, socketID)

	return nil
}

// =============================================================================
// Remote Procedure Call (RPC) - Simplified
// =============================================================================

// RPCRequest represents an RPC request
type RPCRequest struct {
	ID         int
	Method     string
	Parameters []interface{}
	respChan   chan RPCResponse // per-call channel; routes response to the correct caller
}

// RPCResponse represents an RPC response
type RPCResponse struct {
	RequestID int
	Result    interface{}
	Error     error
}

// RPCServer handles RPC requests
type RPCServer struct {
	methods  map[string]func([]interface{}) (interface{}, error)
	requests chan RPCRequest
	mu       sync.RWMutex
}

// NewRPCServer creates an RPC server
func NewRPCServer() *RPCServer {
	return &RPCServer{
		methods:  make(map[string]func([]interface{}) (interface{}, error)),
		requests: make(chan RPCRequest, 100),
	}
}

// Register registers an RPC method
func (rs *RPCServer) Register(methodName string, handler func([]interface{}) (interface{}, error)) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.methods[methodName] = handler
}

// Call makes an RPC call. Safe for concurrent use — each call receives its
// own response channel so responses are never delivered to the wrong caller.
func (rs *RPCServer) Call(method string, params []interface{}) (interface{}, error) {
	respChan := make(chan RPCResponse, 1)
	req := RPCRequest{
		ID:         int(time.Now().UnixNano()),
		Method:     method,
		Parameters: params,
		respChan:   respChan,
	}

	rs.requests <- req
	resp := <-respChan
	return resp.Result, resp.Error
}

// Serve starts serving RPC requests
func (rs *RPCServer) Serve() {
	go func() {
		for req := range rs.requests {
			rs.mu.RLock()
			handler, exists := rs.methods[req.Method]
			rs.mu.RUnlock()

			var resp RPCResponse
			resp.RequestID = req.ID

			if !exists {
				resp.Error = errors.New("method not found")
			} else {
				result, err := handler(req.Parameters)
				resp.Result = result
				resp.Error = err
			}

			req.respChan <- resp
		}
	}()
}

// Stop stops the RPC server
func (rs *RPCServer) Stop() {
	close(rs.requests)
}
