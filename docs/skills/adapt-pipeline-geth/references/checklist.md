# Standard Geth Adaptation Checklist

Use this checklist to verify the Pipeline adaptation is complete. Search the target codebase for each item.

## Required (Live Tracer Mode)

- [ ] `core/tracing/hooks.go` — `CommitHook` type defined
- [ ] `core/tracing/hooks.go` — `BlockDBStartHook` type defined
- [ ] `core/tracing/hooks.go` — `OnCommit` field in `Hooks` struct
- [ ] `core/tracing/hooks.go` — `OnBlockDBStart` field in `Hooks` struct
- [ ] `core/state/statedb.go` — `onCommit` field in `StateDB` struct
- [ ] `core/state/statedb.go` — `SetOnCommit` method exists
- [ ] `core/state/statedb.go` — `onCommit` invoked in `commitAndFlush`
- [ ] `core/types/receipt.go` — `SetEffectiveGasPrice` method exists
- [ ] `core/state_processor.go` — `SetEffectiveGasPrice` called before `OnTxEnd`
- [ ] `core/blockchain.go` — `OnBlockDBStart` called in `ProcessBlock`
- [ ] `core/blockchain.go` — `SetOnCommit` called in `ProcessBlock`
- [ ] `core/blockchain.go` — Kafka push in `writeBlockAndSetHead`
- [ ] `core/blockchain.go` — Kafka push in `SetCanonical`
- [ ] `core/blockchain.go` — `getCommonAncestor` method exists
- [ ] `core/blockchain.go` — `GetHeaderByHash2` method exists
- [ ] `eth/tracers/live/pipeline.go` — File exists and registers "pipeline"
- [ ] `go.mod` — Contains `github.com/Chaintable/pipeline`

## Optional (RPC Tracer Mode)

- [ ] `core/state/statedb.go` — `StateDiff` method exists
- [ ] `eth/api_debank.go` — File exists with `DebankBlock` method
- [ ] `eth/backend.go` — `trace` namespace registered in `APIs()`

## Build Verification

- [ ] `go build ./...` succeeds with no errors
- [ ] `go vet ./...` reports no issues

## Verification Commands

```bash
# Check hook types
grep -n "CommitHook" $TARGET/core/tracing/hooks.go
grep -n "BlockDBStartHook" $TARGET/core/tracing/hooks.go

# Check StateDB modifications
grep -n "SetOnCommit" $TARGET/core/state/statedb.go
grep -n "StateDiff" $TARGET/core/state/statedb.go

# Check Receipt fix
grep -n "SetEffectiveGasPrice" $TARGET/core/types/receipt.go
grep -n "SetEffectiveGasPrice" $TARGET/core/state_processor.go

# Check blockchain hooks
grep -n "OnBlockDBStart" $TARGET/core/blockchain.go
grep -n "getCommonAncestor" $TARGET/core/blockchain.go

# Check live tracer
grep -rn "pipeline" $TARGET/eth/tracers/live/

# Check RPC
grep -n "DebankBlock" $TARGET/eth/api_debank.go
grep -n "NewDebankAPI" $TARGET/eth/backend.go

# Check dependency
grep "Chaintable/pipeline" $TARGET/go.mod
```
