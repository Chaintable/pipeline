# Adapting Legacy go-ethereum (without Live Tracer support) for Pipeline Integration

This guide is based on the adaptation from `v1.101315.1` in the op-geth project. It describes how to integrate Pipeline into an older go-ethereum fork that uses the legacy `EVMLogger` interface instead of the modern `tracing.Hooks`-based live tracer system.

> **When to use this guide vs. the standard guide:**
>
> | Feature | Standard Guide (go-ethereum v1.16+) | This Guide (Legacy) |
> |---------|--------------------------------------|----------------------|
> | Tracer interface | `tracing.Hooks` (struct-based) | `vm.EVMLogger` (interface-based) |
> | Registration | `tracers.LiveDirectory.Register()` | `vm.Config.Tracer` (direct assignment) |
> | Hook delivery | Built-in hook dispatch | Manual hook dispatch in `state_processor.go` |
> | Pipeline code | External Go module dependency | Embedded source code (copied into project) |
> | Log capture | Native `OnLog` hook | Injected via `statedb.SetHooks()` |
> | Call tracing | `OnEnter`/`OnExit` hooks | `CaptureStart`/`CaptureEnd`/`CaptureEnter`/`CaptureExit` |
> | Opcode tracing | `OnOpcode` hook | `CaptureState` method |
> | Block lifecycle | Dispatched by framework | Manually called in `insertChain` |
> | Tracer disable in non-tracing paths | Not needed (hook-based) | Must set `Tracer = nil` in miner, prefetcher, API |

---

## Key Differences from the Standard Adaptation

### 1. Embedded Pipeline Code Instead of External Dependency

Legacy clients cannot use `go get github.com/Chaintable/pipeline` because the internal package paths differ. Instead, the entire pipeline codebase is **copied into the project** under `pipeline/`:

```
pipeline/
├── leader/
│   ├── config.go
│   ├── leader_failover.go
│   └── manager.go
├── metrics/
│   └── metrics.go
├── processor/
│   ├── push.go
│   └── serializer.go
├── tracer/
│   ├── call_tracer.go
│   ├── pipeline.go
│   ├── pipeline_tracer.go
│   └── prestate_tracer.go
├── types/
│   ├── block.go
│   ├── block_file.go
│   ├── block_notification.go
│   ├── event.go
│   ├── state_diff.go
│   ├── trace.go
│   └── transaction.go
├── util/
│   ├── codec.go
│   ├── file.go
│   ├── kafka.go
│   ├── s3.go
│   ├── to_hash.go
│   └── tracer.go
└── writer/
    └── registry.go
```

All import paths use the project's module path (e.g., `github.com/ethereum/go-ethereum/pipeline/tracer`) instead of `github.com/Chaintable/pipeline/tracer`.

### 2. EVMLogger Interface Instead of tracing.Hooks

The `PipelineTracer` must implement the `vm.EVMLogger` interface:

```go
type EVMLogger interface {
    CaptureTxStart(gasLimit uint64)
    CaptureTxEnd(restGas uint64)
    CaptureStart(env *EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int)
    CaptureEnd(output []byte, gasUsed uint64, err error)
    CaptureEnter(typ OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int)
    CaptureExit(output []byte, gasUsed uint64, err error)
    CaptureState(pc uint64, op OpCode, gas, cost uint64, scope *ScopeContext, rData []byte, depth int, err error)
    CaptureFault(pc uint64, op OpCode, gas, cost uint64, scope *ScopeContext, depth int, err error)
}
```

### 3. Manual Hook Dispatch

Since the legacy framework does not automatically dispatch hooks like `OnTxStart`, `OnTxEnd`, `OnBlockStart`, `OnBlockEnd`, these must be **manually called** in the appropriate places (`state_processor.go`, `blockchain.go`).

---

## Adaptation Overview

| File | Change Type | Description |
|------|-------------|-------------|
| `pipeline/**` | New directory | Embedded pipeline source code |
| `core/tracing/hooks.go` | New file | Custom hooks definition (simplified) |
| `core/state/statedb.go` | Modify | Add `SetHooks`, `OnLog` dispatch, `OnCommit` in `Commit` |
| `core/types/receipt.go` | Add method | Add `SetEffectiveGasPrice` |
| `core/state_processor.go` | Modify | Manually dispatch `OnTxStart`, `OnTxEnd`, `OnSystemCallStartHookV2` |
| `core/state_transition.go` | Modify | Handle deposit tx failure tracing |
| `core/blockchain.go` | Modify | Tracer init, hook building, block lifecycle, Kafka push |
| `core/genesis.go` | Add function | Add `getGenesisState` helper |
| `eth/backend.go` | Modify | Create `PipelineTracer` from config |
| `eth/ethconfig/config.go` | Modify | Add `VMTrace` and `VMTraceJsonConfig` fields |
| `cmd/utils/flags.go` | Modify | Add `--vmtrace` and `--vmtrace.jsonconfig` CLI flags |
| `cmd/geth/main.go` | Modify | Register new CLI flags |
| `miner/worker.go` | Modify | Disable tracer during block building |
| `eth/api_backend.go` | Modify | Disable tracer for API EVM calls |
| `go.mod` | Modify | Add dependencies (aws-sdk, kafka, etcd, etc.) |

---

## Step 1: Embed Pipeline Source Code

Copy the entire pipeline codebase into the project under `pipeline/`. Update all import paths from `github.com/Chaintable/pipeline/...` to `github.com/ethereum/go-ethereum/pipeline/...` (or whatever the project's module path is).

Add the required dependencies to `go.mod`:

```bash
go get github.com/aws/aws-sdk-go-v2@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/credentials@latest
go get github.com/aws/aws-sdk-go-v2/service/s3@latest
go get github.com/klauspost/compress
go get go.etcd.io/etcd/client/v3
go get github.com/segmentio/kafka-go
```

---

## Step 2: Create Tracing Hooks (core/tracing/hooks.go)

Since the legacy go-ethereum does not have `core/tracing/hooks.go`, create it as a **new file**. This is a simplified version compared to the standard guide - it only defines the types needed by the pipeline:

```go
package tracing

import (
    "math/big"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/params"
    "github.com/holiman/uint256"
)

// StateDB gives tracers access to the whole state.
type StateDB interface {
    GetBalance(common.Address) *uint256.Int
    GetNonce(common.Address) uint64
    GetCode(common.Address) []byte
    GetCodeHash(common.Address) common.Hash
    GetState(common.Address, common.Hash) common.Hash
    GetTransientState(common.Address, common.Hash) common.Hash
    Exist(common.Address) bool
    GetRefund() uint64
}

// VMContext provides the context for the EVM execution.
type VMContext struct {
    Coinbase    common.Address
    BlockNumber *big.Int
    Time        uint64
    Random      *common.Hash
    BaseFee     *big.Int
    StateDB     StateDB
}

type (
    TxStartHook         = func(vmContext *VMContext, tx *types.Transaction, from common.Address)
    TxEndHook           = func(receipt *types.Receipt, err error)
    BlockchainInitHook  = func(chainConfig *params.ChainConfig)
    CloseHook           = func()
    BlockStartHook      = func(block *types.Block)
    BlockEndHook        = func(err error)
    GenesisBlockHook    = func(genesis *types.Block, alloc types.GenesisAlloc)
    CommitHook          = func(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, accountsOrigin map[common.Address][]byte, storages map[common.Hash]map[common.Hash][]byte, storagesOrigin map[common.Address]map[common.Hash][]byte, codes map[common.Hash][]byte)
    LogHook             = func(log *types.Log)
    OnSystemCallStartHookV2 = func(vm *VMContext)
)

type Hooks struct {
    OnTxStart               TxStartHook
    OnTxEnd                 TxEndHook
    OnBlockchainInit        BlockchainInitHook
    OnClose                 CloseHook
    OnBlockStart            BlockStartHook
    OnSystemCallStartHookV2 OnSystemCallStartHookV2
    OnBlockEnd              BlockEndHook
    OnGenesisBlock          GenesisBlockHook
    OnLog                   LogHook
    OnCommit                CommitHook
}
```

**Key differences from the standard guide:**
- No `OnEnter`, `OnExit`, `OnOpcode`, `OnBalanceChange`, `OnBlockDBStart` hooks
- These are handled by the `EVMLogger` interface (`CaptureStart`, `CaptureEnter`, `CaptureState`, etc.) which the framework dispatches automatically
- `OnLog` is injected via `statedb.SetHooks()` since the legacy framework doesn't dispatch log hooks
- Added `OnSystemCallStartHookV2` for Optimism system call tracing

---

## Step 3: Implement PipelineTracer as EVMLogger

The `PipelineTracer` must implement `vm.EVMLogger`. The key mapping between legacy and modern interfaces:

| Legacy EVMLogger Method | Modern Hook Equivalent | Notes |
|------------------------|----------------------|-------|
| `CaptureStart` | `OnEnter` (depth=0) | Top-level call frame |
| `CaptureEnd` | `OnExit` (depth=0) | Top-level call exit |
| `CaptureEnter` | `OnEnter` (depth>0) | Nested call frames |
| `CaptureExit` | `OnExit` (depth>0) | Nested call exits |
| `CaptureState` | `OnOpcode` | Opcode-level tracing |
| `CaptureFault` | (part of `OnOpcode`) | Opcode fault |
| `CaptureTxStart` | (unused) | Not needed |
| `CaptureTxEnd` | (unused) | Not needed |

The `PipelineTracer` also provides a `BuildHooks` function to create a `tracing.Hooks` struct for hooks that are NOT part of `EVMLogger`:

```go
func BuildHooks(t *PipelineTracer) *tracing.Hooks {
    return &tracing.Hooks{
        OnBlockchainInit: t.OnBlockchainInit,
        OnClose:          t.OnClose,
        OnBlockStart:     t.OnBlockStart,
        OnTxStart:        t.OnTxStart,
        OnTxEnd:          t.OnTxEnd,
        OnLog:            t.OnLog,
        OnGenesisBlock:   t.OnGenesisBlock,
        OnCommit:         t.OnCommit,
    }
}
```

---

## Step 4: Modify StateDB (core/state/statedb.go)

### 4.1 Add hooks Field and SetHooks Method

```go
// In StateDB struct:
hooks *tracing.Hooks

// New method:
func (s *StateDB) SetHooks(hooks *tracing.Hooks) {
    s.hooks = hooks
}
```

### 4.2 Dispatch OnLog in AddLog

Since the legacy framework does not dispatch log hooks, inject it in `AddLog`:

```go
func (s *StateDB) AddLog(log *types.Log) {
    // ... existing code ...
    log.TxIndex = uint(s.txIndex)
    log.Index = s.logSize
    if s.hooks != nil && s.hooks.OnLog != nil {
        s.hooks.OnLog(log)
    }
    s.logs[s.thash] = append(s.logs[s.thash], log)
    s.logSize++
}
```

### 4.3 Invoke OnCommit in Commit

In the `Commit` method, after processing dirty state objects, collect the codes and call `OnCommit`:

```go
// Collect codes during dirty object processing
codes := make(map[common.Hash][]byte)
for addr := range s.stateObjectsDirty {
    obj := s.stateObjects[addr]
    if obj.code != nil && obj.dirtyCode {
        rawdb.WriteCode(codeWriter, common.BytesToHash(obj.CodeHash()), obj.code)
        codes[common.BytesToHash(obj.CodeHash())] = obj.code  // <-- collect
        obj.dirtyCode = false
    }
    // ...
}

// After computing root, before trie update:
destructs := s.convertAccountSet(s.stateObjectsDestruct)
if s.hooks != nil && s.hooks.OnCommit != nil {
    s.hooks.OnCommit(origin, root, destructs, s.accounts, nil, s.storages, nil, codes)
}
```

**Key difference from standard guide:** The legacy `Commit` method has a different structure than `commitAndFlush`. The state data fields (`s.accounts`, `s.storages`, `s.stateObjectsDestruct`) are accessed differently. Also, `accountsOrigin` and `storagesOrigin` are passed as `nil` since the legacy StateDB may not have these fields.

---

## Step 5: Fix Receipt EffectiveGasPrice (core/types/receipt.go)

Same as the standard guide:

```go
func (r *Receipt) SetEffectiveGasPrice(tx *Transaction, baseFee *big.Int) {
    r.EffectiveGasPrice = tx.inner.effectiveGasPrice(new(big.Int), baseFee)
}
```

---

## Step 6: Manually Dispatch Hooks in StateProcessor (core/state_processor.go)

This is a **major difference** from the standard guide. In the legacy framework, `OnTxStart`, `OnTxEnd`, and system call hooks are NOT automatically dispatched. They must be manually added to `Process`:

```go
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (...) {
    // ... existing setup ...

    // Extract pipeline tracer
    var pipelineTracer *tracer.PipelineTracer
    if p, ok := cfg.Tracer.(*tracer.PipelineTracer); !ok {
        log.Crit("vmConfig.Tracer must be a pipeline.Tracer")
    } else {
        pipelineTracer = p
    }

    // Handle system calls (e.g., beacon block root) with tracing
    if beaconRoot := block.BeaconRoot(); beaconRoot != nil {
        pipelineTracer.OnSystemCallStartHookV2(&tracing.VMContext{
            Coinbase: header.Coinbase,
            StateDB:  statedb,
        })
        ProcessBeaconBlockRoot(*beaconRoot, vmenv, statedb)
    }

    // Set hooks on statedb for log capture
    statedb.SetHooks(tracer.BuildHooks(pipelineTracer))

    for i, tx := range block.Transactions() {
        // ... existing message setup ...

        // Manually dispatch OnTxStart
        if pipelineTracer != nil {
            pipelineTracer.OnTxStart(&tracing.VMContext{
                Coinbase: header.Coinbase,
                StateDB:  statedb,
            }, tx, msg.From)
        }

        receipt, err := applyTransaction(...)

        // Manually dispatch OnTxEnd with effective gas price
        if pipelineTracer != nil {
            receipt.SetEffectiveGasPrice(tx, vmenv.Context.BaseFee)
            // For L2 chains (e.g., Optimism): include L1 fee in effective gas price
            if vmenv.Context.L1CostFunc != nil && !tx.IsDepositTx() && receipt.GasUsed > 0 {
                l1Fee := vmenv.Context.L1CostFunc(tx.RollupCostData(), vmenv.Context.Time)
                if l1Fee != nil && l1Fee.Cmp(common.Big0) > 0 {
                    gasUsed := new(big.Int).SetUint64(receipt.GasUsed)
                    receipt.EffectiveGasPrice = new(big.Int).Div(
                        new(big.Int).Add(l1Fee, new(big.Int).Mul(receipt.EffectiveGasPrice, gasUsed)),
                        gasUsed,
                    )
                }
            }
            pipelineTracer.OnTxEnd(receipt, err)
        }

        // ... rest of transaction processing ...
    }
}
```

**L2-specific note:** For Optimism (and similar L2s), the effective gas price must include the L1 data fee. This is calculated using `L1CostFunc` and added to the base effective gas price.

---

## Step 7: Handle Deposit Transaction Failures (core/state_transition.go)

For Optimism deposit transactions that fail, ensure the tracer captures the exit:

```go
if err != nil && err != ErrGasLimitReached && st.msg.IsDepositTx {
    if st.evm.Config.Tracer != nil {
        st.evm.Config.Tracer.CaptureEnter(vm.STOP, common.Address{}, common.Address{}, nil, 0, nil)
    }
    st.state.RevertToSnapshot(snap)
    // ...
}
```

---

## Step 8: Modify BlockChain (core/blockchain.go)

### 8.1 Add hooks Field to BlockChain

```go
type BlockChain struct {
    // ... existing fields ...
    hooks *tracing.Hooks
}
```

### 8.2 Initialize Hooks in NewBlockChain

After blockchain initialization, build hooks from the tracer:

```go
if vmConfig.Tracer != nil {
    if _, ok := vmConfig.Tracer.(*tracer.PipelineTracer); !ok {
        log.Crit("vmConfig.Tracer must be a pipeline.Tracer")
    } else {
        bc.hooks = tracer.BuildHooks(vmConfig.Tracer.(*tracer.PipelineTracer))
    }
}

// Dispatch blockchain init and genesis hooks
if bc.hooks != nil && bc.hooks.OnBlockchainInit != nil {
    bc.hooks.OnBlockchainInit(chainConfig)
}
if bc.hooks != nil && bc.hooks.OnGenesisBlock != nil {
    if block := bc.CurrentBlock(); block.Number.Uint64() == 0 {
        alloc, err := getGenesisState(bc.db, block.Hash())
        // ...
        bc.hooks.OnGenesisBlock(bc.genesisBlock, alloc)
    }
}
```

### 8.3 Manually Dispatch OnBlockStart/OnBlockEnd in insertChain

In the legacy framework, `OnBlockStart` and `OnBlockEnd` are NOT automatically called. Add them around `Process`:

```go
// In insertChain, around the block processing:
if bc.hooks != nil && bc.hooks.OnBlockStart != nil {
    bc.hooks.OnBlockStart(block)
}
receipts, logs, usedGas, err := bc.processor.Process(block, statedb, bc.vmConfig)
if bc.hooks != nil && bc.hooks.OnBlockEnd != nil {
    bc.hooks.OnBlockEnd(err)
}
```

### 8.4 Disable Tracer in Prefetcher

The prefetcher runs blocks speculatively and must NOT trigger the pipeline tracer:

```go
go func(start time.Time, followup *types.Block, throwaway *state.StateDB) {
    vmConfig := bc.vmConfig
    vmConfig.Tracer = nil  // <-- disable tracer
    bc.prefetcher.Prefetch(followup, throwaway, vmConfig, &followupInterrupt)
}
```

### 8.5 Dispatch OnClose in Stop

```go
func (bc *BlockChain) Stop() {
    // ... existing cleanup ...
    if bc.hooks != nil && bc.hooks.OnClose != nil {
        bc.hooks.OnClose()
    }
}
```

### 8.6 Add Kafka Push and Helper Methods

Same as the standard guide: add Kafka push logic in `writeBlockAndSetHead` and `SetCanonical`, add `getCommonAncestor` and `GetHeaderByHash2` helper methods. These use `bc.hooks.OnCommit` instead of `bc.logger.OnCommit`.

---

## Step 9: Add getGenesisState Helper (core/genesis.go)

```go
func getGenesisState(db ethdb.Database, blockhash common.Hash) (alloc types.GenesisAlloc, err error) {
    blob := rawdb.ReadGenesisStateSpec(db, blockhash)
    if len(blob) != 0 {
        if err := alloc.UnmarshalJSON(blob); err != nil {
            return nil, err
        }
        return alloc, nil
    }
    // Fallback to known networks
    var genesis *Genesis
    switch blockhash {
    case params.MainnetGenesisHash:
        genesis = DefaultGenesisBlock()
    case params.SepoliaGenesisHash:
        genesis = DefaultSepoliaGenesisBlock()
    // ... add other known networks ...
    }
    if genesis != nil {
        return genesis.Alloc, nil
    }
    return nil, nil
}
```

---

## Step 10: Create PipelineTracer from Config (eth/backend.go)

Unlike the standard guide which uses `tracers.LiveDirectory.Register()`, the legacy approach creates the tracer directly:

```go
func New(stack *node.Node, config *ethconfig.Config) (*Ethereum, error) {
    // ... existing code ...
    if config.VMTrace != "" {
        traceConfig := json.RawMessage("{}")
        if config.VMTraceJsonConfig != "" {
            traceConfig = json.RawMessage(config.VMTraceJsonConfig)
        }
        t, err := tracer.NewPipelineTracer(traceConfig)
        if err != nil {
            return nil, fmt.Errorf("failed to create tracer %s: %v", config.VMTrace, err)
        }
        vmConfig.Tracer = t  // Assign as EVMLogger
    }
    // ...
}
```

---

## Step 11: Add CLI Flags (cmd/utils/flags.go + cmd/geth/main.go)

### 11.1 Define Flags

```go
VMTraceFlag = &cli.StringFlag{
    Name:     "vmtrace",
    Usage:    "Name of tracer which should record internal VM operations (costly)",
    Category: flags.VMCategory,
}
VMTraceJsonConfigFlag = &cli.StringFlag{
    Name:     "vmtrace.jsonconfig",
    Usage:    "Tracer configuration (JSON)",
    Value:    "{}",
    Category: flags.VMCategory,
}
```

### 11.2 Wire Flags to Config

In `SetEthConfig`:

```go
if ctx.IsSet(VMTraceFlag.Name) {
    if name := ctx.String(VMTraceFlag.Name); name != "" {
        cfg.VMTrace = name
        cfg.VMTraceJsonConfig = ctx.String(VMTraceJsonConfigFlag.Name)
    }
}
```

### 11.3 Add Config Fields (eth/ethconfig/config.go)

```go
type Config struct {
    // ... existing fields ...
    VMTrace           string
    VMTraceJsonConfig string
}
```

---

## Step 12: Disable Tracer in Non-Tracing Paths

The tracer must be disabled in paths that are NOT block import to avoid incorrect data:

### 12.1 Miner/Worker (miner/worker.go)

```go
func (w *worker) applyTransaction(env *environment, tx *types.Transaction) (*types.Receipt, error) {
    vmcfg := *w.chain.GetVMConfig()
    vmcfg.Tracer = nil  // <-- disable
    receipt, err := core.ApplyTransaction(..., vmcfg)
}
```

### 12.2 API Backend (eth/api_backend.go)

```go
func (b *EthAPIBackend) GetEVM(...) *vm.EVM {
    vcfg := *vmConfig
    vcfg.Tracer = nil  // <-- disable
    return vm.NewEVM(context, txContext, state, b.ChainConfig(), vcfg)
}
```

---

## Adaptation Checklist

### Required

- [ ] `pipeline/**` - Embed pipeline source code with updated import paths
- [ ] `core/tracing/hooks.go` - Create custom hooks file (new file)
- [ ] `core/state/statedb.go` - Add `hooks` field, `SetHooks`, `OnLog` dispatch, `OnCommit` in `Commit`
- [ ] `core/types/receipt.go` - Add `SetEffectiveGasPrice`
- [ ] `core/state_processor.go` - Manually dispatch `OnTxStart`, `OnTxEnd`, system call hooks
- [ ] `core/blockchain.go` - Add `hooks` field, init hooks, dispatch `OnBlockStart`/`OnBlockEnd`
- [ ] `core/blockchain.go` - Add Kafka push in `writeBlockAndSetHead` and `SetCanonical`
- [ ] `core/blockchain.go` - Add `getCommonAncestor` and `GetHeaderByHash2`
- [ ] `core/blockchain.go` - Dispatch `OnClose` in `Stop`
- [ ] `core/genesis.go` - Add `getGenesisState` helper
- [ ] `eth/backend.go` - Create `PipelineTracer` from config
- [ ] `eth/ethconfig/config.go` - Add `VMTrace` and `VMTraceJsonConfig`
- [ ] `cmd/utils/flags.go` - Add `--vmtrace` and `--vmtrace.jsonconfig` flags
- [ ] `cmd/geth/main.go` - Register flags
- [ ] `miner/worker.go` - Disable tracer during block building
- [ ] `eth/api_backend.go` - Disable tracer for API calls
- [ ] `go.mod` - Add dependencies (aws-sdk, kafka, etcd, etc.)

### Tracer Disable Paths (Critical)

- [ ] `miner/worker.go` - `applyTransaction`: set `Tracer = nil`
- [ ] `eth/api_backend.go` - `GetEVM`: set `Tracer = nil`
- [ ] `core/blockchain.go` - prefetcher goroutine: set `Tracer = nil`

### L2-Specific (Optimism)

- [ ] `core/state_processor.go` - Include L1 fee in effective gas price
- [ ] `core/state_transition.go` - Handle failed deposit tx tracing
- [ ] `core/state_processor.go` - Dispatch `OnSystemCallStartHookV2` before beacon root processing

### Optional

- [ ] `cmd/geth/dbcmd.go` - Add `db set-finalized` command
- [ ] `Dockerfile.debank` - Build image
- [ ] `.github/workflows/build.debank.yml` - CI pipeline

---

## Summary: Standard vs Legacy Adaptation

```
Standard (go-ethereum v1.16+)          Legacy (older forks like op-geth)
─────────────────────────────          ──────────────────────────────────
Pipeline as Go module dependency  ──►  Pipeline source embedded in project
tracing.Hooks live tracer system  ──►  vm.EVMLogger interface
Framework dispatches all hooks    ──►  Manual dispatch of block/tx hooks
OnEnter/OnExit for calls          ──►  CaptureStart/CaptureEnd/CaptureEnter/CaptureExit
OnOpcode for opcodes              ──►  CaptureState
Built-in OnLog dispatch           ──►  statedb.SetHooks() + manual dispatch
No need to disable tracer         ──►  Must disable in miner, API, prefetcher
commitAndFlush has stateUpdate    ──►  Commit has different internal structure
```
