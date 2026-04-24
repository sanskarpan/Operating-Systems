# FEATURES

## Phase 1 — Memory Management Extensions

- [x] **MEM-001** | `memory/` | Demand paging simulation — page fault handler fetches from a mock disk, evicts via existing replacement algorithms, retries the access
- [x] **MEM-002** | `memory/` | Working set model — Δ-window tracking; pages outside the working set are candidates for eviction; expose `WorkingSetSize()` and thrashing detection
- [x] **MEM-003** | `memory/` | Buddy system allocator — power-of-2 block splitting and coalescing; `Allocate(size)` / `Free(ptr, size)`; internal + external fragmentation metrics
- [x] **MEM-004** | `memory/` | Two-level page table — PD + PT hierarchy; `Translate(virtualAddr)` → physical frame; sparse address space support

## Phase 2 — CPU Scheduling Extensions

- [x] **SCHED-001** | `scheduling/` | New package — EDF (Earliest Deadline First) scheduler for periodic real-time tasks; deadline miss detection and reporting
- [x] **SCHED-002** | `scheduling/` | Rate Monotonic (RM) scheduler — fixed-priority assignment by period; Liu & Layland utilization bound check (U ≤ n(2^(1/n) − 1))
- [x] **SCHED-003** | `scheduling/` | CFS (Completely Fair Scheduler) — virtual runtime with red-black tree (use a sorted structure); per-task vruntime, min-vruntime tracking, weight/nice levels
- [x] **SCHED-004** | `scheduling/` | Work-stealing scheduler — per-worker deques; idle workers steal from the back of busy workers' queues; configurable steal threshold

## Phase 3 — Networking Package (new)

- [x] **NET-001** | `networking/` | TCP finite state machine — all 11 states (CLOSED → LISTEN → SYN_SENT → SYN_RECEIVED → ESTABLISHED → FIN_WAIT_1/2 → TIME_WAIT → CLOSE_WAIT → LAST_ACK → CLOSING); correct transition table
- [x] **NET-002** | `networking/` | Sliding window flow control — sender window, receiver window, window scaling; simulate packet loss and retransmit
- [x] **NET-003** | `networking/` | ARQ protocols — Stop-and-Wait, Go-Back-N, Selective Repeat; configurable loss rate; throughput and efficiency metrics
- [x] **NET-004** | `networking/` | Nagle's algorithm — coalesce small segments; disable flag (TCP_NODELAY equivalent); compare throughput with/without
- [x] **NET-005** | `networking/` | Basic routing table — longest-prefix match; add/delete routes; `Route(destIP) → nextHop`

## Phase 4 — Lock-Free Concurrency Package (new)

- [x] **LF-001** | `lockfree/` | Treiber stack — CAS-based push/pop; ABA problem demonstration and fix using tagged pointers
- [x] **LF-002** | `lockfree/` | Michael & Scott queue — two-CAS non-blocking enqueue/dequeue; separate head/tail pointers
- [x] **LF-003** | `lockfree/` | SPSC ring buffer — single-producer single-consumer; power-of-2 size; cache-line padding to eliminate false sharing
- [x] **LF-004** | `lockfree/` | MPMC ring buffer — multi-producer multi-consumer with sequence numbers (Dmitry Vyukov's algorithm)
- [x] **LF-005** | `lockfree/` | Futex simulation — fast path in userspace (atomic CAS); slow path via channel-based wait/wake; benchmark vs sync.Mutex

## Phase 5 — Filesystem Extensions

- [x] **FS-001** | `filesystem/` | Write-ahead log (WAL) journaling — log entry before applying; crash recovery by replaying or discarding uncommitted log entries
- [x] **FS-002** | `filesystem/` | Hard links & reference counting — inode refcount; `Link()` / `Unlink()` semantics; file deleted only when refcount reaches 0
- [x] **FS-003** | `filesystem/` | Buffer cache — LRU block cache in front of BlockManager; dirty tracking; write-back flush; hit rate metrics
- [x] **FS-004** | `filesystem/` | Copy-on-write (CoW) — shared read-only pages on fork; fault on write triggers private copy; snapshot support

## Phase 6 — IPC Extensions

- [x] **IPC-001** | `ipc/` | Named pipes (FIFOs) — persistent rendezvous point by name; open blocks until both reader and writer are ready; supports multiple readers
- [x] **IPC-002** | `ipc/` | Pub-Sub broker — topic registry; `Subscribe(topic)` returns a channel; `Publish(topic, msg)` fan-outs to all subscribers with backpressure handling
- [x] **IPC-003** | `ipc/` | Ordered message passing — FIFO ordering per sender; causal ordering via vector clocks; total ordering via Lamport timestamps

## Phase 7 — Advanced Patterns

- [x] **PAT-001** | `patterns/` | Singleflight — deduplicate concurrent identical in-flight requests; one goroutine executes, all callers share the result
- [x] **PAT-002** | `patterns/` | Saga pattern — distributed transaction with sequential steps and compensating rollback actions on failure
- [x] **PAT-003** | `patterns/` | Backpressure — producer pauses or drops when consumer buffer is full; configurable strategies (drop oldest, drop newest, block, error)
- [x] **PAT-004** | `patterns/` | Bulkhead — isolated goroutine/semaphore pool per resource type; prevents one resource's failures from starving others
- [x] **PAT-005** | `patterns/` | Scatter-gather — fan out request to N upstreams; collect first K responses and cancel the rest; timeout-aware
- [x] **PAT-006** | `patterns/` | Work stealing — per-worker double-ended queue; steal from back when idle; compare throughput vs simple shared queue

## Phase 8 — Concurrency Classic Problems Extensions

- [x] **CONC-001** | `concurrency/` | Santa Claus problem — 9 reindeer + elves; correct semaphore solution; demonstrate starvation-free scheduling
- [x] **CONC-002** | `concurrency/` | Livelock detector — instrument goroutines; detect retry loops making no progress; report livelock within configurable window
- [x] **CONC-003** | `concurrency/` | Priority inversion demo + priority inheritance fix — show unbounded PI; implement priority inheritance and priority ceiling protocols

## ✅ Final Status

All 35 features implemented and verified. Completed 2026-04-24.

```
go test ./... -race -count=1 -timeout 180s   → ok (12/12 packages)
go vet ./...                                  → clean
go build ./...                                → clean
```

### Summary by phase

| Phase | Items | Package(s) |
|-------|-------|-----------|
| 1 — Memory Extensions        | MEM-001..004   | `memory/`       |
| 2 — CPU Scheduling           | SCHED-001..004 | `scheduling/`   |
| 3 — Networking               | NET-001..005   | `networking/`   |
| 4 — Lock-Free Concurrency    | LF-001..005    | `lockfree/`     |
| 5 — Filesystem Extensions    | FS-001..004    | `filesystem/`   |
| 6 — IPC Extensions           | IPC-001..003   | `ipc/`          |
| 7 — Advanced Patterns        | PAT-001..006   | `patterns/`     |
| 8 — Classic Problems         | CONC-001..003  | `concurrency/`  |
