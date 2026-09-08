# Legacy Geth Adaptation Checklist

Use this checklist to verify the Pipeline adaptation is complete. Replace `$TARGET` with the target repository path.

## Embedded Pipeline Code

- [ ] `pipeline/` directory exists with all subpackages
- [ ] Import paths match the target module path (not `github.com/Chaintable/pipeline/`)
- [ ] `pipeline/tracer/pipeline_tracer.go` implements `vm.EVMLogger`
- [ ] `go build ./pipeline/...` succeeds

## Core Tracing

- [ ] `core/tracing/hooks.go` exists (new file)
- [ ] `StateDB` interface defined
- [ ] `VMContext` struct defined
- [ ] All hook types defined: `TxStartHook`, `TxEndHook`, `BlockchainInitHook`, `CloseHook`, `BlockStartHook`, `BlockEndHook`, `GenesisBlockHook`, `CommitHook`, `LogHook`
- [ ] `Hooks` struct defined with all fields

## StateDB Modifications

- [ ] `core/state/statedb.go` — `hooks` field in `StateDB` struct
- [ ] `core/state/statedb.go` — `SetHooks` method
- [ ] `core/state/statedb.go` — `OnLog` dispatch in `AddLog`
- [ ] `core/state/statedb.go` — `OnCommit` invoked in `Commit`

## Receipt Fix

- [ ] `core/types/receipt.go` — `SetEffectiveGasPrice` method

## StateProcessor (Manual Hook Dispatch)

- [ ] `core/state_processor.go` — Extracts `PipelineTracer` from `cfg.Tracer`
- [ ] `core/state_processor.go` — Calls `statedb.SetHooks()`
- [ ] `core/state_processor.go` — Dispatches `OnTxStart` before each tx
- [ ] `core/state_processor.go` — Calls `SetEffectiveGasPrice` before `OnTxEnd`
- [ ] `core/state_processor.go` — Dispatches `OnTxEnd` after each tx

## BlockChain Modifications

- [ ] `core/blockchain.go` — `hooks` field in `BlockChain` struct
- [ ] `core/blockchain.go` — Hooks initialized in `NewBlockChain`
- [ ] `core/blockchain.go` — `OnBlockchainInit` dispatched
- [ ] `core/blockchain.go` — `OnGenesisBlock` dispatched for genesis
- [ ] `core/blockchain.go` — `OnBlockStart` before `Process` in `insertChain`
- [ ] `core/blockchain.go` — `OnBlockEnd` after `Process` in `insertChain`
- [ ] `core/blockchain.go` — `OnClose` in `Stop`
- [ ] `core/blockchain.go` — Tracer disabled in prefetcher
- [ ] `core/blockchain.go` — Kafka push in `writeBlockAndSetHead`
- [ ] `core/blockchain.go` — Kafka push in `SetCanonical`
- [ ] `core/blockchain.go` — `getCommonAncestor` method
- [ ] `core/blockchain.go` — `GetHeaderByHash2` method

## Config and CLI

- [ ] `eth/ethconfig/config.go` — `VMTrace` field
- [ ] `eth/ethconfig/config.go` — `VMTraceJsonConfig` field
- [ ] `eth/backend.go` — Creates `PipelineTracer` from config
- [ ] `cmd/utils/flags.go` — `VMTraceFlag` defined
- [ ] `cmd/utils/flags.go` — `VMTraceJsonConfigFlag` defined
- [ ] `cmd/utils/flags.go` — Flags wired in `SetEthConfig`
- [ ] `cmd/geth/main.go` — Flags registered

## Tracer Disable Paths (Critical)

- [ ] `miner/worker.go` — `Tracer = nil` in `applyTransaction`
- [ ] `eth/api_backend.go` — `Tracer = nil` in `GetEVM`
- [ ] `core/blockchain.go` — `Tracer = nil` in prefetcher goroutine

## L2-Specific (if applicable)

- [ ] `core/state_processor.go` — L1 fee included in effective gas price
- [ ] `core/state_transition.go` — Failed deposit tx tracing handled
- [ ] `core/state_processor.go` — `OnSystemCallStartHookV2` dispatched

## Dependencies

- [ ] `go.mod` — aws-sdk-go-v2
- [ ] `go.mod` — klauspost/compress
- [ ] `go.mod` — etcd client v3
- [ ] `go.mod` — segmentio/kafka-go

## Build Verification

- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` reports no issues

## Verification Commands

```bash
# Check embedded pipeline
ls $TARGET/pipeline/tracer/ $TARGET/pipeline/types/ $TARGET/pipeline/util/

# Check import paths
grep -rn "Chaintable/pipeline" $TARGET/pipeline/  # Should return NOTHING

# Check hooks file
test -f $TARGET/core/tracing/hooks.go && echo "OK" || echo "MISSING"

# Check StateDB modifications
grep -n "SetHooks" $TARGET/core/state/statedb.go
grep -n "OnCommit" $TARGET/core/state/statedb.go
grep -n "hooks.OnLog" $TARGET/core/state/statedb.go

# Check manual hook dispatch
grep -n "OnTxStart" $TARGET/core/state_processor.go
grep -n "OnTxEnd" $TARGET/core/state_processor.go

# Check blockchain hooks
grep -n "OnBlockStart" $TARGET/core/blockchain.go
grep -n "OnBlockEnd" $TARGET/core/blockchain.go
grep -n "OnClose" $TARGET/core/blockchain.go

# Check tracer disable
grep -n "Tracer = nil" $TARGET/miner/worker.go
grep -n "Tracer = nil" $TARGET/eth/api_backend.go

# Check CLI flags
grep -n "VMTraceFlag" $TARGET/cmd/utils/flags.go
```
