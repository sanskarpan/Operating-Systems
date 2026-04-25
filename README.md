# Operating Systems

Comprehensive OS concepts implemented in Go — no external dependencies, full race-detector coverage.

```
go test ./... -race   →   12/12 packages pass
go vet ./...          →   clean
go build ./...        →   clean
```

---

## Packages

| Package | Concepts |
|---------|----------|
| [`memory/`](#memory) | Demand paging, Working set, Buddy allocator, Two-level page table |
| [`scheduling/`](#scheduling) | EDF, Rate Monotonic, CFS, Work-stealing |
| [`networking/`](#networking) | TCP FSM, Sliding window, ARQ, Nagle, Routing |
| [`lockfree/`](#lockfree) | Treiber stack, M&S queue, SPSC/MPMC rings, Futex |
| [`filesystem/`](#filesystem) | WAL journaling, Hard links, Buffer cache, Copy-on-write |
| [`ipc/`](#ipc) | Named pipes, Pub-sub broker, Ordered messaging |
| [`patterns/`](#patterns) | Singleflight, Saga, Backpressure, Bulkhead, Scatter-gather, Work-stealing + channels, circuit breaker, rate limiting, worker pools |
| [`concurrency/`](#concurrency) | Santa Claus, Livelock detector, Priority inversion + inheritance + PCP |
| [`deadlock/`](#deadlock) | Banker's algorithm, RAG cycle detection, Wait-for graph |
| [`process/`](#process) | PCB, FCFS/SJF/RR/MLFQ schedulers, fork/exec |
| [`sync/`](#sync) | Mutex, Semaphore, RWLock, Barrier, Monitor, Peterson's algorithm |
| [`io/`](#io) | FCFS/SSTF/SCAN/C-SCAN/LOOK disk scheduling |

---

## Memory

### MEM-001 — Demand Paging
```go
disk := NewDiskStore()
sim := NewDemandPagingSimulator(4, disk) // 4 physical frames
fault, evicted := sim.Access(pageNum)
sim.Write(pageNum, data)
fmt.Println(sim.PageFaultRate())
```
LRU frame pool; dirty-page write-back on eviction; fully concurrent-safe.

### MEM-002 — Working Set Model
```go
ws := NewWorkingSetModel(delta=10, frames=4)
ws.Access(page)
fmt.Println(ws.WorkingSetSize()) // distinct pages in last Δ refs
fmt.Println(ws.IsThrashing())   // WSS > frames
```

### MEM-003 — Buddy Allocator
```go
b, _ := NewBuddyAllocator(totalSize=1024, minBlockSize=16)
addr, _ := b.Allocate(100)  // rounds up to 128
b.Free(addr, 100)           // XOR buddy coalescing
fmt.Println(b.ExternalFragmentation())
```

### MEM-004 — Two-Level Page Table
```go
pt := NewTwoLevelPageTable(pdBits=10, ptBits=10, offsetBits=12, frameCount=1024)
pt.Map(virtualAddr, frameNum)
physAddr, err := pt.Translate(virtualAddr)
```
Sparse: page-table pages allocated on demand. Metrics via `sync/atomic`.

---

## Scheduling

### SCHED-001 — EDF (Earliest Deadline First)
```go
tasks := []*RTTask{{ID: 0, Period: 4, Execution: 1}, ...}
sched := NewEDFScheduler(tasks)
trace := sched.Run(20) // returns per-tick task IDs
fmt.Println(sched.UtilizationCheck(), sched.DeadlineMisses)
```

### SCHED-002 — Rate Monotonic
```go
sched := NewRMScheduler(tasks) // assigns priority: shorter period → higher
fmt.Println(sched.UtilizationBound()) // n(2^(1/n)−1)
```

### SCHED-003 — CFS
```go
s := NewCFSScheduler(timeslice=1.0)
s.AddTask(&CFSTask{ID: "A", Nice: 0})
running := s.Tick(wallClock)
fmt.Println(s.Fairness()) // CV of vruntimes; lower = fairer
```

### SCHED-004 — Work-Stealing
```go
ws := NewWSScheduler(numWorkers=4, stealThreshold=3)
ws.Start()
id := ws.Submit(func() { /* work */ })
ws.Stop()
fmt.Println(ws.Stolen)
```

---

## Networking

### NET-001 — TCP Finite State Machine
```go
conn := NewTCPConnection()
conn.Event(EventPassiveOpen) // → LISTEN
conn.Event(EventSYN)         // → SYN_RECEIVED
conn.Event(EventACK)         // → ESTABLISHED
```
All 11 RFC-793 states; complete immutable transition table; History audit trail.

### NET-002 — Sliding Window
```go
sender := NewSlidingWindowSender(windowSize=4, lossRate=0.1, seed=42)
seqNum, dropped, err := sender.Send(data)
sender.Ack(ackNum)
seg, _ := sender.Retransmit(seqNum)
```

### NET-003 — ARQ
```go
_, m := StopAndWait(frames=20, lossRate=0.3, seed=42)
_, m := GoBackN(frames=20, windowSize=4, lossRate=0.2, seed=42)
_, m := SelectiveRepeat(frames=20, windowSize=4, lossRate=0.2, seed=42)
fmt.Println(m.Efficiency) // delivered / total transmissions
```

### NET-004 — Nagle's Algorithm
```go
n := NewNagleBuffer(mss=1460)
segs := n.Write([]byte("hi"))    // sent (no outstanding)
segs  = n.Write([]byte("x"))     // buffered (small + outstanding)
segs  = n.Ack(2)                 // releases buffer
n.SetNoDelay(true)               // TCP_NODELAY
```

### NET-005 — Routing Table
```go
rt := NewRoutingTable()
rt.AddRoute(mustParseIP("10.0.0.0"), MaskFromPrefix(8),  "gw1")
rt.AddRoute(mustParseIP("10.1.0.0"), MaskFromPrefix(16), "gw2")
route, _ := rt.Route(mustParseIP("10.1.2.3")) // → gw2 (LPM)
```

---

## Lock-Free

### LF-001 — Treiber Stack (ABA-safe)
```go
s := NewTreiberStack()
s.Push(val)
v, ok := s.Pop()
```
Tagged pointer (ptr + version counter) packed via `unsafe.Pointer` CAS.

### LF-002 — Michael & Scott Queue
```go
q := NewMSQueue()
q.Enqueue(val)
v, ok := q.Dequeue()
```

### LF-003 — SPSC Ring Buffer
```go
r, _ := NewSPSCRingBuffer(size=1024) // must be power-of-2
r.TryPush(val)
v, ok := r.TryPop()
```
Cache-line padded head and tail to eliminate false sharing.

### LF-004 — MPMC Ring Buffer (Vyukov)
```go
q, _ := NewMPMCRingBuffer(capacity=1024)
q.TryEnqueue(val)
v, ok := q.TryDequeue()
```

### LF-005 — Futex
```go
f := NewFutex()
f.Lock()    // fast CAS(0→1); slow path via channel with missed-wake fix
f.Unlock()  // transfers ownership — never sets state=0 when handing off
f.TryLock() bool
f.WaitTimeout(50 * time.Millisecond) bool
fmt.Println(f.FastAcquires, f.SlowAcquires)
```

---

## Filesystem

### FS-001 — WAL Journaling
```go
store := NewWALStore()
store.Put("key", []byte("value")) // log → apply → commit
store.Delete("key")
fresh := store.Recover()          // replay committed entries only
```

### FS-002 — Hard Links
```go
fs := NewHardLinkFS()
fs.Create("a.txt", data)
fs.Link("a.txt", "b.txt")        // same inode, refcount=2
fs.Unlink("a.txt")               // refcount=1; data intact
fs.Unlink("b.txt")               // refcount=0; data freed
```

### FS-003 — Buffer Cache (LRU)
```go
bc := NewBufferCache(bm, capacity=64)
data, _ := bc.Read(blockNum)
bc.Write(blockNum, data)
bc.Flush()
fmt.Println(bc.HitRate())
```

### FS-004 — Copy-on-Write
```go
m := NewCoWManager()
m.AllocatePage(pid=1, pageNum=0, data)
m.Fork(parent=1, child=2)
m.Write(child, 0, newData)    // CoW fault: private copy for child
fmt.Println(m.WriteFaults)
snap := m.Snapshot(pid=1)
```

---

## IPC

### IPC-001 — Named Pipes
```go
reg := NewPipeRegistry()
reg.Create("myfifo", bufSize=8)
go func() { reader, _ := reg.OpenRead("myfifo"); reader.Read() }()
writer, _ := reg.OpenWrite("myfifo") // blocks until reader connects
writer.Write([]byte("hello"))
```

### IPC-002 — Pub-Sub Broker
```go
b := NewPubSubBroker()
ch := b.Subscribe("events", bufSize=32)
b.Publish("events", msg)       // fan-out; drops on full (backpressure)
fmt.Println(b.TotalDropped)
```

### IPC-003 — Ordered Messaging
```go
om := NewOrderedMessenger()
om.Register(1); om.Register(2)
om.Send(1, "hello")                    // stamped with Lamport TS + vector clock
msgs, _ := om.Receive(2)              // delivered in Lamport order, FIFO per sender
```

---

## Patterns

### PAT-001 — Singleflight
```go
g := NewSingleflightGroup()
val, shared, err := g.Do("key", expensiveFn) // N callers → 1 execution
```

### PAT-002 — Saga
```go
result := NewSaga().
    AddStep("book-hotel",  bookHotel,  cancelHotel).
    AddStep("book-flight", bookFlight, cancelFlight).
    Run()
// On failure: compensations run in reverse order
```

### PAT-003 — Backpressure
```go
b := NewBackpressureBuffer(capacity=100, DropOldest)
b.Send(item)    // evicts oldest if full
b.Send(item)    // DropNewest | Block | ErrorOnFull also available
```

### PAT-004 — Bulkhead
```go
bh := NewBulkhead("database", maxConcurrent=10, waitTimeout=50*time.Millisecond)
err := bh.Execute(func() error { return db.Query() })
// rejected with error if pool exhausted within timeout
```

### PAT-005 — Scatter-Gather
```go
res := ScatterGather(ctx, upstreams, k=3, timeout=100*time.Millisecond)
// fans out to all upstreams; returns first 3 successes; cancels rest
```

### PAT-006 — Work-Stealing (patterns layer)
```go
e := NewStealingExecutor(workers=4)
e.Start()
e.Submit(workerID, NewStealTask(fn))
e.Stop()
fmt.Println(e.Executed, e.Stolen)
```

---

## Concurrency

### CONC-001 — Santa Claus Problem
```go
sim := NewSantaSimulation()
go sim.RunSanta(stop)
go sim.RunReindeer(id, rounds, vacationMax)
go sim.RunElf(id, times, thinkMax)
// Reindeer have priority; elves served in groups of 3; starvation-free
```

### CONC-002 — Livelock Detector
```go
m := NewLivelock("spinner")
d := NewLivelockDetector(window=10*time.Millisecond, minRetries=5)
d.Register(m); d.Start()
// In goroutine: m.Retry() on each failed attempt; m.Progress() on success
d.ForceCheck()
fmt.Println(d.Detected()) // ["spinner"] if retrying without progress
```

### CONC-003 — Priority Inversion + Fixes
```go
// Priority inheritance
sim := NewPriorityInversionSimulator(inheritance=true)
// When High blocks on mutex held by Low: Low gets High's priority
// Medium cannot preempt Low during inversion window

// Priority Ceiling Protocol
sys := NewPCPSystem()
sys.AddMutex(NewPCPMutex("m", ceiling=PriorityHigh))
ok, err := sys.TryLockPCP(task, mutex)
// blocked if task.priority >= any ceiling held by others
```

---

## Structure

```
.
├── concurrency/   # Classic problems: dining philosophers, readers-writers, sleeping barber, Santa Claus, livelock, priority inversion
├── deadlock/      # Banker's algorithm, RAG, wait-for graph
├── examples/      # Runnable demos
├── filesystem/    # Allocation strategies + WAL, hard links, buffer cache, CoW
├── io/            # Disk scheduling algorithms
├── ipc/           # Message queues, shared memory, signals, sockets, RPC + named pipes, pub-sub, ordered messaging
├── lockfree/      # Treiber stack, M&S queue, SPSC/MPMC rings, Futex
├── memory/        # Page replacement + demand paging, working set, buddy allocator, two-level page table
├── networking/    # TCP FSM, sliding window, ARQ, Nagle, routing
├── patterns/      # Channels, circuit breaker, context, error groups, rate limiting, worker pools + singleflight, saga, backpressure, bulkhead, scatter-gather, work-stealing
├── process/       # PCB, schedulers, fork/exec
├── scheduling/    # EDF, Rate Monotonic, CFS, work-stealing
└── sync/          # Mutex, semaphore, RWLock, barrier, monitor, Peterson's
```

## Running

```bash
# All tests with race detector
go test ./... -race

# Specific package
go test ./lockfree/ -race -v

# Benchmarks
go test ./lockfree/ -bench=. -benchmem

# Examples
go run examples/os_tutorial.go
```
