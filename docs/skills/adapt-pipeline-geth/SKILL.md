---
name: adapt-pipeline-geth
description: "Adapt a go-ethereum v1.14.0+ fork to integrate Pipeline (Live Tracer + RPC Tracer). Requires tracing.Hooks support."
user-invocable: true
argument-hint: "<path-to-geth-fork>"
---

# Standard Geth Pipeline Adaptation (7 Phases)

You are adapting a **standard go-ethereum v1.14.0+** fork to integrate the Pipeline data processing system. This client uses the modern `tracing.Hooks`-based live tracer system.

**Target repository**: `$ARGUMENTS`
**Reference document**: Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` for detailed code examples.
**Pipeline repo**: The current working directory contains the Pipeline source code.

## Important Principles

1. **Explore before modifying** — Always read the target file first to understand its current structure
2. **Incremental verification** — Run `go build ./...` after each phase
3. **Adapt, don't copy blindly** — The reference code is based on go-ethereum v1.16.7; the target may have different struct layouts, method signatures, or import paths
4. **Ask when uncertain** — If the target code structure differs significantly from the reference, ask the user before proceeding

---

## Phase 1: Extend Tracing Hooks

### Goal
Add `CommitHook` and `BlockDBStartHook` types to the tracing hooks system.

### Explore
1. Read `$ARGUMENTS/core/tracing/hooks.go`
2. Find the existing hook type definitions (e.g., `BalanceChangeReason`, `BlockHashReadHook`)
3. Find the `Hooks` struct definition

### Reference
Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` — Step 1 for exact type signatures.

### Modify
1. **Add hook type definitions** after the existing hook types:
   - `CommitHook` — called when state is committed, receives origin root, new root, destructs, accounts, accountsOrigin, storages, storagesOrigin, codes
   - `BlockDBStartHook` — called before block processing, receives a `StateDB` interface

2. **Add fields to Hooks struct**:
   - `OnCommit CommitHook`
   - `OnBlockDBStart BlockDBStartHook`

### Verify
```bash
cd $ARGUMENTS && go build ./core/tracing/...
```

---

## Phase 2: Modify StateDB

### Goal
Add commit callback support and a `StateDiff` method for RPC Tracer mode.

### Explore
1. Read `$ARGUMENTS/core/state/statedb.go`
2. Find the `StateDB` struct definition
3. Find the `commitAndFlush` method (or equivalent commit method)
4. Check what fields are available in the commit result (look for `stateUpdate`, `ret`, or similar)
5. Check existing imports (need `rlp` and `tracing`)

### Reference
Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` — Step 2 for code examples.

### Modify
1. **Add `onCommit` field** to `StateDB` struct: `onCommit tracing.CommitHook`
2. **Add `SetOnCommit` method**: sets the onCommit callback
3. **Insert onCommit invocation** in `commitAndFlush`:
   - After snapshot commit, before trie database commit
   - Collect codes, split accounts into destructs vs updates
   - Call `s.onCommit(...)` with all state change data
4. **Add `StateDiff` method** (for RPC Tracer mode):
   - Computes intermediate root
   - Iterates `stateObjectsDestruct` for destructs
   - Iterates `mutations` for account/storage changes
   - Returns root, destructs, accounts, storages, codes

### Verify
```bash
cd $ARGUMENTS && go build ./core/state/...
```

---

## Phase 3: Fix Receipt EffectiveGasPrice

### Goal
Ensure `receipt.EffectiveGasPrice` is populated before `OnTxEnd` fires.

### Explore
1. Read `$ARGUMENTS/core/types/receipt.go` — find the `Receipt` struct
2. Read `$ARGUMENTS/core/state_processor.go` — find where `OnTxEnd` is called (search for `OnTxEnd`)
3. Check if `EffectiveGasPrice` field exists on `Receipt`

### Reference
Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` — Step 3.

### Modify
1. **Add `SetEffectiveGasPrice` to Receipt**:
   - Computes effective gas price from transaction and base fee
   - Uses `tx.inner.effectiveGasPrice(new(big.Int), baseFee)`
2. **Modify `OnTxEnd` dispatch in state_processor.go**:
   - Before calling `hooks.OnTxEnd(receipt, err)`, call `receipt.SetEffectiveGasPrice(tx, evm.Context.BaseFee)`

### Verify
```bash
cd $ARGUMENTS && go build ./core/types/... ./core/...
```

---

## Phase 4: Modify BlockChain

### Goal
Integrate Pipeline hooks into the block processing lifecycle and add Kafka push logic.

### Explore
1. Read `$ARGUMENTS/core/blockchain.go`
2. Find `ProcessBlock` method — locate where `OnBlockEnd` is set up
3. Find `writeBlockAndSetHead` method — locate where `ChainEvent` is sent
4. Find `SetCanonical` method (if exists)
5. Check if `bc.logger` exists and what type it is (should be `*tracing.Hooks`)

### Reference
Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` — Step 4 for all code.

### Modify
1. **Add imports**: `pipeline/leader`, `pipeline/tracer`, `pipeline/util`, `pipeline/types`
2. **Inject hooks in ProcessBlock**:
   - Call `OnBlockDBStart(statedb)` before block processing
   - Call `statedb.SetOnCommit(bc.logger.OnCommit)` to inject commit callback
3. **Add Kafka push in writeBlockAndSetHead**:
   - Check leader status
   - Compute common ancestor for reorg detection
   - Build `BlockChangeNotification`
   - Handle empty blocks (parent.Root == block.Root)
   - Push notification via `tracer.NodeXPusher`
4. **Add same Kafka push logic in SetCanonical**
5. **Add helper methods**:
   - `getCommonAncestor` — walks block ancestry to find common ancestor
   - `GetHeaderByHash2` — falls back to S3 download if header not found locally

### Verify
```bash
cd $ARGUMENTS && go build ./core/...
```

---

## Phase 5: Register Live Tracer

### Goal
Create a new file that registers Pipeline as a go-ethereum live tracer.

### Explore
1. Read `$ARGUMENTS/eth/tracers/live/` directory — see existing live tracers for pattern reference
2. Check import paths (module name from `go.mod`)
3. Verify `tracers.LiveDirectory` is available

### Reference
Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` — Step 5.

### Modify
Create `$ARGUMENTS/eth/tracers/live/pipeline.go`:
- Package `live`
- `init()` registers "pipeline" via `tracers.LiveDirectory.Register`
- `NewPipelineTracer` creates the tracer from JSON config and returns `*tracing.Hooks`
- Map all pipeline hooks: `OnBlockchainInit`, `OnClose`, `OnBlockStart`, `OnTxStart`, `OnTxEnd`, `OnEnter`, `OnExit`, `OnLog`, `OnOpcode`, `OnGenesisBlock`, `OnCommit`, `OnBalanceChange`, `OnBlockDBStart`

### Verify
```bash
cd $ARGUMENTS && go build ./eth/tracers/...
```

---

## Phase 6: Implement RPC Tracer

### Goal
Add the `trace_debankBlock` RPC endpoint for on-demand block tracing.

### Explore
1. Read `$ARGUMENTS/eth/backend.go` — find the `APIs()` method
2. Read existing API files in `$ARGUMENTS/eth/` for patterns
3. Check how blocks are fetched and state is accessed

### Reference
Read `docs/skills/adapt-pipeline-geth/references/adaptation-guide.md` — Step 6.

### Modify
1. **Create `$ARGUMENTS/eth/api_debank.go`**:
   - `DebankAPI` struct holding `*Ethereum` reference
   - `DebankBlock` method implementing `trace_debankBlock`:
     - Fetch target block
     - Handle genesis block specially
     - Get parent state
     - Create `RPCTracer` and register hooks
     - Replay block via `Processor.Process`
     - Call `statedb.StateDiff()` for state changes
     - Verify state root
     - Return `rpcTracer.GetOutPut()`
2. **Register in `$ARGUMENTS/eth/backend.go`**:
   - Add `{Namespace: "trace", Service: NewDebankAPI(s)}` to `APIs()`

### Verify
```bash
cd $ARGUMENTS && go build ./eth/...
```

---

## Phase 7: Add Dependencies and Final Verification

### Goal
Add the Pipeline module dependency and verify the complete build.

### Explore
1. Read `$ARGUMENTS/go.mod` — check existing dependencies and module path
2. Check if any indirect dependencies might conflict

### Modify
1. Add Pipeline dependency:
   ```bash
   cd $ARGUMENTS && go get github.com/Chaintable/pipeline@latest
   ```
2. Run `go mod tidy` to clean up

### Final Verification
```bash
cd $ARGUMENTS && go build ./...
```

If build fails, analyze errors and fix them. Common issues:
- Import path mismatches
- Missing method implementations due to interface changes
- Struct field name differences between go-ethereum versions

---

## Completion

After all 7 phases are complete, present a summary:

```
## Adaptation Complete

### Files Modified:
- core/tracing/hooks.go — Added CommitHook, BlockDBStartHook
- core/state/statedb.go — Added onCommit, SetOnCommit, StateDiff
- core/types/receipt.go — Added SetEffectiveGasPrice
- core/state_processor.go — Modified OnTxEnd to set gas price
- core/blockchain.go — Added hook injection, Kafka push, helpers

### Files Created:
- eth/tracers/live/pipeline.go — Live tracer registration
- eth/api_debank.go — RPC tracer endpoint

### Dependencies Added:
- github.com/Chaintable/pipeline

### Next Steps:
1. Configure pipeline JSON (see docs/skills/adapt-pipeline-geth/references/adaptation-guide.md — Configuration Example)
2. Start geth with `--vmtrace pipeline --vmtrace.jsonconfig '{...}'`
3. Test RPC with `curl -X POST --data '{"method":"trace_debankBlock","params":["0x1"]}'`
```

Run the checklist from `references/checklist.md` to verify completeness.
