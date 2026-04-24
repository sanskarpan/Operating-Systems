# ISSUES

## 🔴 Critical Bugs (correctness/safety)

- **BUG-001** | `sync/sync.go:68` | `AcquireTimeout` cannot enforce its deadline — the timer goroutine closes a channel but cannot interrupt `sync.Cond.Wait()`, so the caller may block indefinitely if `Release()` is never called | Status: [x] Fixed — rewrote to spawn a single goroutine that calls `Broadcast()` on expiry so `Wait()` returns and the loop can check the deadline

- **BUG-002** | `ipc/ipc.go:703` | `RPCServer.Call` is broken under concurrency — all callers share a single `responses` channel with no request-ID routing, so concurrent calls receive each other's responses | Status: [x] Fixed — replaced shared `responses` channel with a per-call `respChan` embedded in `RPCRequest`; removed the now-unnecessary shared channel from the struct

## 🟠 Logic Bugs (behavioral incorrectness)

- **BUG-003** | `ipc/ipc.go:492` | `ProcessSignalTable.SendSignal` appends to `pst.pending` on every call but the slice is never drained, causing unbounded memory growth | Status: [x] Fixed — `deliverSignal` now removes the first matching entry from `pending` under the lock after looking up the handler

- **BUG-004** | `deadlock/deadlock.go:499` | `CanGrantRequest` mutates the RAG (adds an edge, then rolls it back) without holding any lock — concurrent calls will corrupt graph state | Status: [x] Fixed — added `sync.Mutex` to RAG; `AddRequestEdge`, `AddAssignEdge`, and `CanGrantRequest` all lock it; `CanGrantRequest` inlines the edge mutation to avoid re-entrant locking

- **BUG-005** | `sync/sync.go:81` | `AcquireTimeout` leaks one goroutine per loop iteration — sleep goroutines are created inside the retry loop and never cancelled when the semaphore is acquired | Status: [x] Fixed — resolved as part of BUG-001: the new implementation spawns exactly one goroutine per call and cancels it via `defer close(done)`

## 🟡 Code Quality (smells, duplication, anti-patterns)

- **QUALITY-001** | `concurrency/concurrency.go:54,60,71,78` | `fmt.Printf` called while holding `pc.mu` — I/O under a lock causes unnecessary contention and potential priority inversion | Status: [x] Fixed — captured snapshot values under the lock, released lock before printing; also removed "waiting" prints that were emitted while the lock was held

- **QUALITY-002** | `ipc/ipc.go:431` | `SharedMemoryManager.Detach` is a documented no-op — the comment says it should decrement a reference count but the body is empty, silently doing nothing | Status: [x] Fixed — added segment-existence validation (returns error for unknown IDs) and rewrote the doc comment to clearly state this is a simulation with no reference counting

- **QUALITY-003** | `tests/` | Root-level `tests/` directory is empty — misleads contributors into thinking integration tests exist | Status: [x] Fixed — removed empty directory

## 🔵 Missing Tests (untested paths)

- **TEST-001** | `filesystem/filesystem.go` | Entire filesystem module has zero test coverage — no test file exists | Status: [x] Fixed — created `filesystem/filesystem_test.go` with 35 tests covering BlockManager, all three allocation methods, FileSystem CRUD ops, BitVector, FreeBlockList, and ParsePath

- **TEST-002** | `sync/sync.go:68` | `AcquireTimeout` has no test exercising the case where `Release()` is never called — the timeout bug (BUG-001) is not caught | Status: [x] Fixed — added `TestSemaphoreAcquireTimeout` that verifies the call returns within a bounded multiple of the requested timeout when no permits are available

- **TEST-003** | `ipc/ipc.go:703` | `RPCServer.Call` has no concurrent test — the response-routing race (BUG-002) is not exercised | Status: [x] Fixed — added `TestRPCServerConcurrent` that launches 20 concurrent callers and verifies each receives the correct response

## 🔴 Additional Issues Found During Race-Detector Run

- **RACE-001** | `patterns/channels.go:111` | `FanIn` closes the output channel after a fixed 1ms sleep while multiplex goroutines may still be sending — data race on the channel | Status: [x] Fixed — rewrote `FanIn` to use `sync.WaitGroup`; output channel is closed only after all multiplex goroutines exit

- **RACE-002** | `patterns/channels_test.go:116` | `TestTee` reads `results1` from the main goroutine while a goroutine is writing to it — data race on the slice | Status: [x] Fixed — added `sync.WaitGroup` in `TestTee`; main goroutine waits for the collector goroutine via `wg.Wait()` before reading `results1`

- **RACE-003** | `ipc/ipc_test.go:203` | `TestSignalHandler` reads `received` bool from the test goroutine while the handler goroutine writes it — data race on the variable | Status: [x] Fixed — replaced `bool` with `int32` and used `sync/atomic` for all reads and writes

- **BUG-006** | `patterns/context.go:48` | `WithTimeout` calls `fn(ctx)` synchronously — the timeout context is created but never enforced if `fn` ignores it, so the function can block far past the timeout | Status: [x] Fixed — `fn` is now run in a goroutine; `select` on `ctx.Done()` vs the result channel enforces the timeout externally

- **BUG-007** | `patterns/context.go:87` | `WithDeadline` same problem as BUG-006 — deadline is never enforced externally | Status: [x] Fixed — same pattern applied: `fn` runs in goroutine, `select` enforces the deadline

- **BUG-008** | `patterns/circuitbreaker.go:484` | `AdaptiveCircuitBreaker` initialises `window` with all `false` (= all failures) so the error rate is 1.0 on the very first execution, causing the circuit to open immediately | Status: [x] Fixed — `NewAdaptiveCircuitBreaker` now initialises all window entries to `true` so the initial error rate is 0.0

- **BUG-009** | `patterns/context.go:328` | `ContextWorkerPool.results` channel is never closed — `for range pool.ResultsContext()` in tests blocks indefinitely after `Cancel()` is called | Status: [x] Fixed — `StartContext` now tracks workers with `sync.WaitGroup` and closes `results` in a goroutine once all workers exit

## 🟠 Additional Logic Bugs Found During Race-Detector Run

- **BUG-010** | `patterns/context.go:372` | `MergeContexts` goroutine parameter named `ctx` shadows the outer merged context, and both `select` cases watch the same channel — cancellation randomly not propagated | Status: [x] Fixed — renamed outer result to `merged`, parameter to `c`; second case now watches `merged.Done()` to cleanly exit the goroutine

- **BUG-011** | `patterns/ratelimit.go:338` | `SlidingWindowCounter.Allow()` resets `elapsed` to 0 after a window slide, causing `percentageInCurrent=0` and full previous-window weight, so the first request after a window boundary is incorrectly denied | Status: [x] Fixed — after sliding one window, `elapsed` is set to `elapsed - window` and `currentStart` is advanced by exactly one window; requests more than 2 windows old cause a full reset

- **BUG-012** | `patterns/ratelimit.go:494` | `AdaptiveRateLimiter.IncreaseRate()` uses integer division `(maxRate-currentRate)/10` which yields 0 when the gap is <10, so the rate never increases | Status: [x] Fixed — added `if increment < 1 { increment = 1 }` floor to guarantee at least a 1-unit increase

## 🔵 Additional Race Conditions Fixed in Tests

- **RACE-004** | `patterns/errorgroup_test.go` | `TestLimitedErrorGroup` and `TestParallelMapWithLimit` increment/read `running`/`maxRunning` from concurrent goroutines without sync | Status: [x] Fixed — protected with `sync.Mutex`

- **RACE-005** | `patterns/errorgroup_test.go` | `TestHierarchicalErrorGroup`, `TestParallelExecute`, `TestParallelMap`, `TestBatchErrorGroup` concurrently append to shared slices without sync | Status: [x] Fixed — protected with `sync.Mutex`

- **RACE-006** | `patterns/workerpool_test.go` | `TestWorkerPoolConcurrency` increments `counter` from multiple worker goroutines without sync | Status: [x] Fixed — changed to `int64` with `sync/atomic.AddInt64`

- **RACE-007** | `patterns/workerpool_test.go` | `TestBoundedWorkerPoolLimit` increments `running`/`maxRunning` from concurrent goroutines without sync | Status: [x] Fixed — protected with `sync.Mutex`

- **RACE-008** | `patterns/workerpool_test.go` | `TestTaskQueueStop` increments `processed` from concurrent task goroutines without sync | Status: [x] Fixed — changed to `int64` with `sync/atomic.AddInt64`

## ⚪ Minor / Style

- **STYLE-001** | `sync/sync.go:15` | Package is named `sync`, identical to stdlib — unconventional and confusing for IDE navigation and tooling | Status: [x] Fixed — renamed package declaration to `ossync` in both `sync/sync.go` and `sync/sync_test.go`

## ✅ Final Status

All 20 issues resolved. Verified clean on 2026-04-22:

```
go test ./... -race -timeout 120s   → ok (all 11 packages, zero races)
go vet ./...                        → (no output — zero issues)
go build ./...                      → (no output — zero issues)
```

**Summary by category:**
| Category | Count | Status |
|----------|-------|--------|
| 🔴 Critical Bugs | 2 | All fixed |
| 🟠 Logic Bugs | 5 (3 original + BUG-010..012) | All fixed |
| 🟡 Code Quality | 3 | All fixed |
| 🔵 Missing Tests + Races | 8 (3 TEST + RACE-001..008) | All fixed |
| ⚪ Minor / Style | 1 | All fixed |
| **Total** | **20** | **All fixed ✅** |
