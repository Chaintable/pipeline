# Integration Modes

This document describes the two integration modes for Pipeline and the StateDiff acquisition strategies.

## Overview

Pipeline supports two integration modes and two StateDiff acquisition strategies:

| Dimension | Options |
|-----------|---------|
| **Integration Mode** | Live Tracer, RPC Tracer |
| **StateDiff Strategy** | StateDB-based, Opcode-based |

### Priority Order

The recommended priority order for choosing a combination:

| Priority | Combination | When to Use |
|----------|-------------|-------------|
| 1st (Best) | Live Tracer + StateDB-based | Can integrate with block processing AND modify statedb |
| 2nd | RPC Tracer + StateDB-based | Cannot implement Live Tracer, but can modify statedb |
| 3rd | Live Tracer + Opcode-based | Can integrate with block processing, but cannot modify statedb |
| 4th (Last resort) | RPC Tracer + Opcode-based | Cannot implement Live Tracer AND cannot modify statedb |

### Decision Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Integration Strategy Selection                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   Q1: Can you integrate Live Tracer into block processing?              │
│                                                                          │
│              YES                                    NO                   │
│               │                                     │                    │
│               ▼                                     ▼                    │
│   Q2: Can you modify statedb?            Q2: Can you modify statedb?    │
│                                                                          │
│       YES           NO                       YES           NO            │
│        │             │                        │             │            │
│        ▼             ▼                        ▼             ▼            │
│   ┌─────────┐   ┌─────────┐            ┌─────────┐   ┌─────────┐        │
│   │Priority │   │Priority │            │Priority │   │Priority │        │
│   │   1st   │   │   3rd   │            │   2nd   │   │   4th   │        │
│   │         │   │         │            │         │   │         │        │
│   │  Live   │   │  Live   │            │   RPC   │   │   RPC   │        │
│   │ Tracer  │   │ Tracer  │            │ Tracer  │   │ Tracer  │        │
│   │    +    │   │    +    │            │    +    │   │    +    │        │
│   │StateDB  │   │ Opcode  │            │StateDB  │   │ Opcode  │        │
│   │ based   │   │ based   │            │ based   │   │ based   │        │
│   └─────────┘   └─────────┘            └─────────┘   └─────────┘        │
│       ▲                                     ▲                            │
│       │                                     │                            │
│      BEST                          PREFERRED when Live                   │
│                                    Tracer not possible                   │
└─────────────────────────────────────────────────────────────────────────┘
```

**Key Points:**
- **StateDB-based is always preferred** over Opcode-based (higher accuracy, lower overhead)
- **When Live Tracer cannot be implemented**, prefer RPC Tracer + StateDB-based over Live Tracer + Opcode-based
- **Opcode-based is the lowest priority** fallback, only use when statedb modification is not feasible

---

## Integration Modes

### Mode 1: Live Tracer

**File:** `tracer/pipeline_tracer.go`

This mode embeds the tracer directly into the execution client's block processing loop using go-ethereum's `tracing.Hooks` interface.

```
Block Execution → PipelineTracer hooks → Real-time data capture → Upload to S3/Kafka
```

**Characteristics:**
- Real-time processing during block execution
- Automatic upload to S3 and Kafka publishing
- Requires integration with execution client's block processing
- Supports leader election for high availability

**Integration Points:**
```go
// Initialize pipeline
tracer.InitPipeline(region, nodeXBucket, chainTableBucket, brokers, topic, chainID, version, s3TmpDir)

// Optional: Setup leader election
tracer.SetupLeaderElection(etcdEndpoints, electionKey, nodeID, version, isBackup, gracePeriod, writerConfig)

// Create tracer and register hooks
pipelineTracer, _ := tracer.NewPipelineTracer(configJSON)

// Register with execution client
vm.SetTracer(pipelineTracer.Hooks())
```

**EVM Hooks Used:**
- `OnBlockStart` / `OnBlockEnd` - Block lifecycle
- `OnTxStart` / `OnTxEnd` - Transaction lifecycle
- `OnEnter` / `OnExit` - Call frame tracking
- `OnOpcode` - Opcode-level tracing
- `OnLog` - Event capture
- `OnCommit` - State commit (for StateDB-based StateDiff)

---

### Mode 2: RPC Tracer (trace_debankBlock)

**File:** `tracer/rpc_tracer.go`

This mode implements a custom RPC method `trace_debankBlock` that replays blocks and generates trace data on-demand.

```
RPC Request (block number) → Block Replay → RPCTracer → Return DebankOutPut
```

**Characteristics:**
- On-demand tracing via RPC call
- Can be added as an RPC extension without modifying core block processing
- Returns complete trace data in single response
- Suitable for historical block analysis

**RPC Interface:**
```go
// RPC method: trace_debankBlock
// Input: block number or hash
// Output: DebankOutPut

type DebankOutPut struct {
    BlockFile      *BlockFile    `json:"block_file"`
    Header         *Header       `json:"header"`
    StateDiff      hexutil.Bytes `json:"state_diff"`      // RLP encoded
    ValidationHash int64         `json:"validation_hash"`
}
```

**Integration:**
```go
// Register RPC method in your execution client
func (api *DebugAPI) TraceDebankBlock(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*DebankOutPut, error) {
    // Create RPC tracer
    rpcTracer := tracer.NewRPCTracer(configJSON)

    // Replay block with tracer
    // ...

    // Get output
    return rpcTracer.GetOutPut(originRoot, root, destructs, accounts, storages, codes)
}
```

---

## StateDiff Acquisition Strategies

Both integration modes support two strategies for obtaining StateDiff:

### Strategy 1: StateDB-based (Recommended)

Obtains StateDiff from statedb modifications, passed through the `OnCommit` hook or equivalent.

```
StateDB modifications → OnCommit(originRoot, root, destructs, accounts, storages, codes) → BlockStorageDiff
```

**Parameters received:**
- `originRoot` - State root before block execution
- `root` - State root after block execution
- `destructs` - Self-destructed accounts
- `accounts` - Modified account data (balance, nonce, code)
- `storages` - Modified storage slots
- `codes` - Newly deployed contract codes

**Pros:**
- Accurate and complete state diff
- Lower overhead (no opcode-level tracking needed)
- Direct access to all state changes

**Cons:**
- Requires modification to statedb to expose commit parameters

**Implementation:**
```go
// In OnCommit hook (Live Tracer)
func (t *PipelineTracer) OnCommit(originRoot, root common.Hash, destructs, accounts, storages, codes) {
    stateDiff := buildStateDiffFromCommit(originRoot, root, destructs, accounts, storages, codes)
    uploadStateDiff(stateDiff)
}

// In RPC Tracer
func (t *RPCTracer) GetOutPut(originRoot, root common.Hash, destructs, accounts, storages, codes) *DebankOutPut {
    stateDiff := buildStateDiffFromCommit(originRoot, root, destructs, accounts, storages, codes)
    return &DebankOutPut{StateDiff: rlp.Encode(stateDiff), ...}
}
```

---

### Strategy 2: Opcode-based (Fallback - Lowest Priority)

> **Warning:** This is a fallback strategy with the **lowest priority**. Only use when StateDB-based approach is not feasible. It may miss certain state changes and has higher overhead.

Obtains StateDiff by tracking EVM opcodes that access or modify state.

```
EVM Execution → OnOpcode intercepts state ops → prestateTracer records pre/post state → Compute diff
```

**When to use:**
- StateDB modification is not possible in your execution client
- Quick prototyping or testing before implementing StateDB-based approach
- Legacy systems where statedb internals cannot be exposed

**Tracked opcodes:**
| Opcode | State Access |
|--------|--------------|
| `SLOAD` | Storage read |
| `SSTORE` | Storage write |
| `BALANCE` | Balance read |
| `SELFBALANCE` | Balance read |
| `EXTCODEHASH` | Code hash read |
| `EXTCODESIZE` | Code size read |
| `EXTCODECOPY` | Code read |
| `CREATE` | Account creation |
| `CREATE2` | Account creation |
| `SELFDESTRUCT` | Account deletion |

**Pros:**
- No statedb modification required
- Works with any EVM implementation
- Can be used when OnCommit parameters are not available

**Cons (significant limitations):**
- Higher overhead due to opcode-level interception
- **May miss state changes** not triggered by opcodes:
  - Block rewards / coinbase payments
  - Beacon chain withdrawals
  - Protocol-level balance changes
- Less accurate than StateDB-based approach
- More complex to maintain and debug

**Implementation:**
```go
// prestateTracer tracks state during execution
type prestateTracer struct {
    pre     stateMap  // State before execution
    post    stateMap  // State after execution
    created map[common.Address]bool
    deleted map[common.Address]bool
}

// OnOpcode intercepts state-accessing operations
func (t *prestateTracer) OnOpcode(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, ...) {
    switch vm.OpCode(op) {
    case vm.SLOAD, vm.SSTORE:
        t.lookupStorage(caller, slot)
    case vm.BALANCE, vm.EXTCODEHASH:
        t.lookupAccount(addr)
    case vm.CREATE:
        t.lookupAccount(crypto.CreateAddress(caller, nonce))
        t.created[addr] = true
    // ...
    }
}

// After execution, compute diff
func (t *prestateTracer) GetStateDiff() *BlockStorageDiff {
    t.processDiffState()  // Compare pre vs post
    return &BlockStorageDiff{
        NewAccounts: ...,
        DeletedAccounts: ...,
        StorageDiff: ...,
        NewCodes: ...,
    }
}
```

---

## Choosing the Right Combination

**Decision Flow:**

```
1. Can you implement Live Tracer?
   YES → Can you modify statedb?
         YES → Live Tracer + StateDB-based [Priority 1 - BEST]
         NO  → Live Tracer + Opcode-based [Priority 3]

   NO  → Can you modify statedb?
         YES → RPC Tracer + StateDB-based [Priority 2 - PREFERRED]
         NO  → RPC Tracer + Opcode-based [Priority 4 - LAST RESORT]
```

**Recommendation Matrix:**

| Priority | Combination | Scenario | Notes |
|----------|-------------|----------|-------|
| 1st (Best) | Live Tracer + StateDB-based | Can integrate Live Tracer AND modify statedb | Optimal for production |
| 2nd | RPC Tracer + StateDB-based | **Cannot** implement Live Tracer, can modify statedb | Preferred fallback |
| 3rd | Live Tracer + Opcode-based | Can integrate Live Tracer, **cannot** modify statedb | Acceptable |
| 4th (Last resort) | RPC Tracer + Opcode-based | **Cannot** implement Live Tracer AND **cannot** modify statedb | Only when no other option |

> **Important:** When Live Tracer cannot be implemented, **prefer RPC Tracer + StateDB-based** over Live Tracer + Opcode-based. StateDB-based always provides more accurate results than Opcode-based.

---

## Configuration Matrix

```
                        ┌─────────────────────────────────────────────┐
                        │            StateDiff Strategy               │
                        ├─────────────────────┬───────────────────────┤
                        │   StateDB-based     │    Opcode-based       │
                        │   (Preferred)       │    (Fallback only)    │
┌───────────────────────┼─────────────────────┼───────────────────────┤
│ Live Tracer           │  OnCommit hook      │  prestateTracer       │
│ (pipeline_tracer.go)  │  [Priority 1-BEST]  │  [Priority 3]         │
├───────────────────────┼─────────────────────┼───────────────────────┤
│ RPC Tracer            │  Replay with        │  Replay with          │
│ (rpc_tracer.go)       │  commit params      │  prestateTracer       │
│                       │  [Priority 2]       │  [Priority 4-LAST]    │
└───────────────────────┴─────────────────────┴───────────────────────┘
```

**Summary:**
- **Priority 1:** Live Tracer + StateDB-based - Best option for production
- **Priority 2:** RPC Tracer + StateDB-based - Preferred when Live Tracer cannot be implemented
- **Priority 3:** Live Tracer + Opcode-based - Acceptable when statedb modification is not feasible
- **Priority 4:** RPC Tracer + Opcode-based - Last resort only

**Key Insight:** StateDB-based is always preferred over Opcode-based. If you cannot implement Live Tracer but can modify statedb, choose RPC Tracer + StateDB-based (Priority 2) over Live Tracer + Opcode-based (Priority 3).
