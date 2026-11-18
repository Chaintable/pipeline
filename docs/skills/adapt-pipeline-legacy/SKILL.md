---
name: adapt-pipeline-legacy
description: "Adapt a legacy go-ethereum fork (< v1.14.0, using vm.EVMLogger) to integrate Pipeline. For clients like op-geth without tracing.Hooks."
user-invocable: true
argument-hint: "<path-to-legacy-geth-fork>"
---

# Legacy Geth Pipeline Adaptation (12 Phases)

You are adapting a **legacy go-ethereum fork** (e.g., op-geth) that uses the `vm.EVMLogger` interface instead of modern `tracing.Hooks`. This requires embedding Pipeline source code directly and manually dispatching lifecycle hooks.

**Target repository**: `$ARGUMENTS`
**Reference document**: Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` for detailed code examples.
**Pipeline repo**: The current working directory contains the Pipeline source code.

## Important Principles

1. **Explore before modifying** — Always read the target file first
2. **Incremental verification** — Run `go build ./...` after each phase
3. **Import paths must match** — All embedded pipeline imports use the target's module path (from `go.mod`)
4. **Legacy differences matter** — The `commitAndFlush` is called `Commit` here, `stateUpdate` fields differ, hooks must be dispatched manually
5. **L2 awareness** — If this is an L2 client (op-geth), handle deposit transactions and L1 fees

---

## Phase 1: Embed Pipeline Source Code

### Goal
Copy the Pipeline codebase into the target project and adapt import paths.

### Explore
1. Read `$ARGUMENTS/go.mod` — get the module path (e.g., `github.com/ethereum-optimism/op-geth`)
2. Check if `$ARGUMENTS/pipeline/` already exists
3. Read the Pipeline source tree structure from the current directory

### Modify
1. Copy the entire Pipeline source into `$ARGUMENTS/pipeline/`:
   ```
   pipeline/leader/    pipeline/metrics/   pipeline/processor/
   pipeline/tracer/    pipeline/types/     pipeline/util/
   pipeline/writer/
   ```
2. Update ALL import paths in the copied files:
   - Replace `github.com/Chaintable/pipeline/` with `<target-module>/pipeline/`
   - Replace go-ethereum imports if the target uses a different module path
3. **Critical**: The embedded `pipeline/tracer/pipeline_tracer.go` must implement `vm.EVMLogger` interface. Check if the `PipelineTracer` already has `CaptureStart`, `CaptureEnd`, `CaptureEnter`, `CaptureExit`, `CaptureState`, `CaptureFault`, `CaptureTxStart`, `CaptureTxEnd` methods.

### Verify
```bash
cd $ARGUMENTS && go build ./pipeline/...
```

---

## Phase 2: Create Tracing Hooks Structure

### Goal
Create `core/tracing/hooks.go` as a new file since legacy geth doesn't have it.

### Explore
1. Check that `$ARGUMENTS/core/tracing/` does NOT exist
2. Read `$ARGUMENTS/core/vm/logger.go` or similar — understand the existing EVMLogger interface
3. Check available types in `core/types/` (Transaction, Receipt, Block, GenesisAlloc)

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 2 for the complete hooks.go file.

### Modify
Create `$ARGUMENTS/core/tracing/hooks.go` with:
- `StateDB` interface (GetBalance, GetNonce, GetCode, GetCodeHash, GetState, etc.)
- `VMContext` struct (Coinbase, BlockNumber, Time, Random, BaseFee, StateDB)
- Hook type definitions: `TxStartHook`, `TxEndHook`, `BlockchainInitHook`, `CloseHook`, `BlockStartHook`, `BlockEndHook`, `GenesisBlockHook`, `CommitHook`, `LogHook`, `OnSystemCallStartHookV2`
- `Hooks` struct with all hook fields

**Key difference from standard**: No `OnEnter`, `OnExit`, `OnOpcode`, `OnBalanceChange`, `OnBlockDBStart` — these are handled by `EVMLogger` methods.

### Verify
```bash
cd $ARGUMENTS && go build ./core/tracing/...
```

---

## Phase 3: Modify StateDB

### Goal
Add hooks support, OnLog dispatch, and OnCommit callback to StateDB.

### Explore
1. Read `$ARGUMENTS/core/state/statedb.go`
2. Find the `StateDB` struct — check existing fields
3. Find the `Commit` method (NOT `commitAndFlush` — legacy uses `Commit`)
4. Find the `AddLog` method
5. Check what state data is available: `stateObjectsDirty`, `stateObjectsDestruct`, `accounts`, `storages`

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 4.

### Modify
1. **Add `hooks` field** to `StateDB`: `hooks *tracing.Hooks`
2. **Add `SetHooks` method**:
   ```go
   func (s *StateDB) SetHooks(hooks *tracing.Hooks) { s.hooks = hooks }
   ```
3. **Dispatch OnLog in AddLog**:
   - After setting `log.TxIndex` and `log.Index`
   - Before appending to `s.logs`
   - Call `s.hooks.OnLog(log)` if hooks is set
4. **Invoke OnCommit in Commit**:
   - Collect `codes` during dirty object processing (where `WriteCode` is called)
   - After computing root, call `s.hooks.OnCommit(origin, root, destructs, s.accounts, nil, s.storages, nil, codes)`
   - Note: `accountsOrigin` and `storagesOrigin` are passed as `nil` (legacy StateDB may not have these)

### Verify
```bash
cd $ARGUMENTS && go build ./core/state/...
```

---

## Phase 4: Modify StateProcessor

### Goal
Manually dispatch `OnTxStart`, `OnTxEnd`, and system call hooks since the legacy framework doesn't do this automatically.

### Explore
1. Read `$ARGUMENTS/core/state_processor.go`
2. Find the `Process` method
3. Check how transactions are iterated (look for `block.Transactions()`)
4. Check for system calls (beacon root, L2 system calls)
5. Check if this is an L2 (look for `L1CostFunc`, `IsDepositTx`, `RollupCostData`)

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 6.

### Modify
1. **Extract PipelineTracer** from `cfg.Tracer`:
   ```go
   var pipelineTracer *tracer.PipelineTracer
   if p, ok := cfg.Tracer.(*tracer.PipelineTracer); ok {
       pipelineTracer = p
   }
   ```
2. **Handle system calls** (beacon root, etc.):
   - Before `ProcessBeaconBlockRoot`, call `pipelineTracer.OnSystemCallStartHookV2(...)`
3. **Set hooks on statedb**: `statedb.SetHooks(tracer.BuildHooks(pipelineTracer))`
4. **For each transaction**:
   - Before `applyTransaction`: call `pipelineTracer.OnTxStart(vmContext, tx, msg.From)`
   - After `applyTransaction`: call `receipt.SetEffectiveGasPrice(tx, baseFee)`
   - **L2 special**: If `L1CostFunc` exists and tx is not deposit, add L1 fee to effective gas price
   - Call `pipelineTracer.OnTxEnd(receipt, err)`

### Verify
```bash
cd $ARGUMENTS && go build ./core/...
```

---

## Phase 5: Modify BlockChain

### Goal
Add hooks initialization, lifecycle dispatch, prefetcher safety, and Kafka push.

### Explore
1. Read `$ARGUMENTS/core/blockchain.go`
2. Find `BlockChain` struct
3. Find `NewBlockChain` constructor
4. Find `insertChain` method — where blocks are processed
5. Find `writeBlockAndSetHead` method
6. Find `SetCanonical` method (if exists)
7. Find `Stop` method
8. Look for prefetcher goroutine

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 8.

### Modify
1. **Add `hooks` field** to `BlockChain` struct: `hooks *tracing.Hooks`
2. **Initialize in NewBlockChain**:
   - Extract `PipelineTracer` from `vmConfig.Tracer`
   - Call `tracer.BuildHooks(...)` to create hooks
   - Dispatch `OnBlockchainInit(chainConfig)`
   - Dispatch `OnGenesisBlock` if current block is genesis
3. **In insertChain** — wrap block processing:
   - Call `bc.hooks.OnBlockStart(block)` before `Process`
   - Call `bc.hooks.OnBlockEnd(err)` after `Process`
4. **Disable tracer in prefetcher**: Set `vmConfig.Tracer = nil` in the prefetcher goroutine
5. **Dispatch OnClose in Stop**: Call `bc.hooks.OnClose()` in the `Stop` method
6. **Add Kafka push** in `writeBlockAndSetHead` and `SetCanonical`:
   - Same logic as standard guide but use `bc.hooks.OnCommit` instead of `bc.logger.OnCommit`
7. **Add helper methods**: `getCommonAncestor`, `GetHeaderByHash2`

### Verify
```bash
cd $ARGUMENTS && go build ./core/...
```

---

## Phase 6: Fix Receipt EffectiveGasPrice

### Goal
Add `SetEffectiveGasPrice` method to Receipt.

### Explore
1. Read `$ARGUMENTS/core/types/receipt.go`
2. Find the `Receipt` struct — check if `EffectiveGasPrice` field exists
3. Check if `effectiveGasPrice` method exists on transaction inner types

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 5.

### Modify
Add to Receipt:
```go
func (r *Receipt) SetEffectiveGasPrice(tx *Transaction, baseFee *big.Int) {
    r.EffectiveGasPrice = tx.inner.effectiveGasPrice(new(big.Int), baseFee)
}
```

### Verify
```bash
cd $ARGUMENTS && go build ./core/types/...
```

---

## Phase 7: Create PipelineTracer Registration

### Goal
Wire up the PipelineTracer creation from config.

### Explore
1. Read `$ARGUMENTS/eth/backend.go` — find `New()` constructor
2. Read `$ARGUMENTS/eth/ethconfig/config.go` — find `Config` struct
3. Check how `vmConfig` is constructed

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 10.

### Modify
1. **Add config fields** in `eth/ethconfig/config.go`:
   ```go
   VMTrace           string
   VMTraceJsonConfig string
   ```
2. **Create tracer in `eth/backend.go`**:
   - In `New()`, if `config.VMTrace != ""`:
   - Create `PipelineTracer` from `config.VMTraceJsonConfig`
   - Assign to `vmConfig.Tracer`

### Verify
```bash
cd $ARGUMENTS && go build ./eth/...
```

---

## Phase 8: Disable Tracer in Non-Tracing Paths

### Goal
Prevent the Pipeline tracer from firing in miner, API, and prefetcher contexts.

### Explore
1. Read `$ARGUMENTS/miner/worker.go` — find `applyTransaction` or similar
2. Read `$ARGUMENTS/eth/api_backend.go` — find `GetEVM` or EVM creation
3. Confirm prefetcher was handled in Phase 5

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 12.

### Modify
1. **miner/worker.go**: In `applyTransaction`, copy vmConfig and set `Tracer = nil`
2. **eth/api_backend.go**: In `GetEVM`, copy vmConfig and set `Tracer = nil`

### Verify
```bash
cd $ARGUMENTS && go build ./miner/... ./eth/...
```

---

## Phase 9: Add CLI Flags

### Goal
Add `--vmtrace` and `--vmtrace.jsonconfig` command-line flags.

### Explore
1. Read `$ARGUMENTS/cmd/utils/flags.go` — find existing flag definitions
2. Read `$ARGUMENTS/cmd/geth/main.go` (or `cmd/*/main.go`) — find flag registration
3. Find `SetEthConfig` function in flags.go

### Reference
Read `docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md` — Step 11.

### Modify
1. **Define flags** in `cmd/utils/flags.go`:
   - `VMTraceFlag` — string flag for tracer name
   - `VMTraceJsonConfigFlag` — string flag for JSON config
2. **Wire flags to config** in `SetEthConfig`:
   - Read `VMTraceFlag` and `VMTraceJsonConfigFlag`
   - Set `cfg.VMTrace` and `cfg.VMTraceJsonConfig`
3. **Register flags** in `cmd/geth/main.go` (or equivalent entry point)

### Verify
```bash
cd $ARGUMENTS && go build ./cmd/...
```

---

## Phase 10: L2 Special Handling (if applicable)

### Goal
Handle L2-specific features like deposit transactions and L1 fees.

### Explore
1. Search for `IsDepositTx` in the target codebase
2. Search for `L1CostFunc` or `L1Fee`
3. Check if this is an Optimism-based L2

### Skip Condition
If no L2 indicators are found, skip this phase.

### Modify (if L2)
1. **EffectiveGasPrice L1 fee inclusion** (should already be done in Phase 4):
   - When `L1CostFunc` exists and tx is not deposit, add L1 fee to effective gas price
2. **Handle failed deposit transactions** in `core/state_transition.go`:
   - When deposit tx fails, ensure tracer captures the exit via `CaptureEnter(vm.STOP, ...)`
3. **Dispatch OnSystemCallStartHookV2** before system calls (beacon root, etc.)

### Verify
```bash
cd $ARGUMENTS && go build ./core/...
```

---

## Phase 11: Add Dependencies

### Goal
Add all required external dependencies to go.mod.

### Explore
Read `$ARGUMENTS/go.mod` — check existing dependencies.

### Modify
```bash
cd $ARGUMENTS
go get github.com/aws/aws-sdk-go-v2@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/credentials@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
go get github.com/klauspost/compress
go get go.etcd.io/etcd/client/v3
go get github.com/segmentio/kafka-go
go mod tidy
```

### Verify
```bash
cd $ARGUMENTS && go build ./pipeline/...
```

---

## Phase 12: Full Build and Test

### Goal
Verify the complete adaptation compiles and passes basic tests.

### Verify
```bash
cd $ARGUMENTS && go build ./...
cd $ARGUMENTS && go vet ./...
```

If build fails, analyze errors. Common issues:
- **Import path mismatches** — embedded pipeline uses wrong module path
- **Interface mismatch** — `PipelineTracer` doesn't implement all `EVMLogger` methods
- **Struct field differences** — legacy StateDB has different internals
- **Missing methods** — `effectiveGasPrice` may have different signature
- **L2 types** — deposit transaction types may need special imports

---

## Completion

After all 12 phases, present summary:

```
## Adaptation Complete

### Files Created:
- pipeline/** — Embedded pipeline source code
- core/tracing/hooks.go — Custom hooks definition

### Files Modified:
- core/state/statedb.go — hooks, SetHooks, OnLog dispatch, OnCommit
- core/types/receipt.go — SetEffectiveGasPrice
- core/state_processor.go — Manual OnTxStart/OnTxEnd dispatch
- core/blockchain.go — Hooks init, lifecycle, Kafka push
- eth/backend.go — PipelineTracer creation
- eth/ethconfig/config.go — VMTrace config fields
- cmd/utils/flags.go — CLI flags
- cmd/geth/main.go — Flag registration
- miner/worker.go — Tracer disabled
- eth/api_backend.go — Tracer disabled
- go.mod — New dependencies

### Next Steps:
1. Configure pipeline JSON
2. Start with `--vmtrace pipeline --vmtrace.jsonconfig '{...}'`
3. Verify block processing triggers tracer output
```

Run the checklist from `references/checklist.md` to verify completeness.
