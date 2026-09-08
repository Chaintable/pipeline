# Pipeline Architecture

This document describes the architecture of the Pipeline system, a blockchain data processing pipeline for extracting, tracing, and processing Ethereum blockchain data.

## Table of Contents

- [System Overview](#system-overview)
- [Architecture Diagram](#architecture-diagram)
- [Core Modules](#core-modules)
  - [Tracer Module](#tracer-module)
  - [Processor Module](#processor-module)
  - [Types Module](#types-module)
  - [Leader Module](#leader-module)
  - [Writer Module](#writer-module)
  - [Util Module](#util-module)
  - [Metrics Module](#metrics-module)
- [Data Flow](#data-flow)
- [Data Structures](#data-structures)
- [Storage Strategy](#storage-strategy)
- [Leader Election](#leader-election)
- [API Reference](#api-reference)
- [Deployment](#deployment)

---

## System Overview

Pipeline is designed to be embedded within Ethereum execution clients (geth-compatible) to capture real-time blockchain state changes. It uses EVM tracing hooks to intercept block and transaction execution, capturing:

- Block metadata and headers
- Transaction details and receipts
- Internal call traces (CALL, CREATE, DELEGATECALL, etc.)
- Event logs
- State changes (balance, nonce, code, storage)

The captured data is serialized and uploaded to S3 buckets, with block change notifications published to Kafka for downstream consumers.

### Key Features

- **Real-time tracing**: Captures data during EVM execution
- **Complete call traces**: Full call stack with gas, input/output, errors
- **State diff tracking**: Records only changed state, not full state
- **Dual bucket strategy**: Separates internal and external data storage
- **High availability**: Leader election for Kafka publishing
- **Multi-chain support**: Ethereum, Scroll, XDC, Nitro (Arbitrum), L2geth

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Ethereum Node                                │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                      EVM Execution                             │  │
│  │  ┌─────────────────────────────────────────────────────────┐  │  │
│  │  │                  PipelineTracer                          │  │  │
│  │  │  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐  │  │  │
│  │  │  │ CallTracer  │  │PrestateTracer│  │  BlockContext  │  │  │  │
│  │  │  │             │  │              │  │                │  │  │  │
│  │  │  │ - Call tree │  │ - Pre state  │  │ - BlockFile    │  │  │  │
│  │  │  │ - Events    │  │ - Post state │  │ - Header       │  │  │  │
│  │  │  │ - Traces    │  │ - StateDiff  │  │ - Notification │  │  │  │
│  │  │  └─────────────┘  └──────────────┘  └────────────────┘  │  │  │
│  │  └─────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                           Processor                                  │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                        Serializer                                ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐ ││
│  │  │ JSON + gzip  │  │     RLP      │  │      S3 Key Gen        │ ││
│  │  │ (BlockFile,  │  │ (StateDiff)  │  │{chainID}/[ver]/{path}  │ ││
│  │  │  Header)     │  │              │  │  (version optional)    │ ││
│  │  └──────────────┘  └──────────────┘  └────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                      PushProcessor                               ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐ ││
│  │  │  S3 Upload   │  │    Kafka     │  │    Local Cache         │ ││
│  │  │  (Retry)     │  │   Publish    │  │    (Optional)          │ ││
│  │  └──────────────┘  └──────────────┘  └────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘
                          │                    │
              ┌───────────┘                    └───────────┐
              ▼                                            ▼
┌───────────────────────────────────┐    ┌───────────────────────────────────┐
│      NodeX Bucket (Internal)      │    │     ChainTable Bucket (External)  │
│  ┌─────────────────────────────┐  │    │  ┌─────────────────────────────┐  │
│  │{chainID}/[ver]/{hash}/block │  │    │  │  {chainID}/[ver]/{blockID}  │  │
│  │ (Header, JSON+gzip)         │  │    │  │  (BlockFile, JSON+gzip)     │  │
│  ├─────────────────────────────┤  │    │  ├─────────────────────────────┤  │
│  │{chainID}/[ver]/{root}/diff  │  │    │  │{chainID}/[ver]/{h}/{id}     │  │
│  │ (StateDiff, RLP)            │  │    │  │  (Validation, JSON+gzip)    │  │
│  └─────────────────────────────┘  │    │  └─────────────────────────────┘  │
│       [ver] = version (optional)  │    │       [ver] = version (optional)  │
└───────────────────────────────────┘    └───────────────────────────────────┘
                                                          │
                          ┌───────────────────────────────┘
                          ▼
              ┌─────────────────────────┐
              │         Kafka           │
              │  ┌───────────────────┐  │
              │  │BlockChangeNotify  │  │
              │  │ - NewBlocks       │  │
              │  │ - DropBlocks      │  │
              │  └───────────────────┘  │
              └─────────────────────────┘
                          │
                          ▼
              ┌─────────────────────────┐
              │   Downstream Consumers  │
              └─────────────────────────┘
```

---

## Core Modules

### Tracer Module

Location: `tracer/`

The tracer module is the heart of the pipeline, embedded within the EVM to capture execution data.

#### Files

| File | Purpose |
|------|---------|
| `pipeline_tracer.go` | Main orchestrator, implements all EVM hooks (Live Tracer mode) |
| `call_tracer.go` | Tracks call stack and builds trace tree |
| `prestate_tracer.go` | Captures pre/post state for diff calculation |
| `pipeline.go` | Infrastructure: initialization, globals, upload |
| `rpc_tracer.go` | RPC debug interface tracer (trace_debankBlock mode) |

> **See Also:** [Integration Modes](integration-modes.md) for detailed documentation on:
> - Live Tracer vs RPC Tracer integration modes
> - StateDB-based vs Opcode-based StateDiff acquisition strategies

#### PipelineTracer

The main tracer that coordinates all tracing activities:

```go
type PipelineTracer struct {
    config         pipelineTracerConfig
    callTracer     *callTracer
    prestateTracer *prestateTracer
}
```

**EVM Hooks:**

| Hook | Trigger | Action |
|------|---------|--------|
| `OnBlockStart` | Block execution begins | Initialize BlockCtx, create BlockFile |
| `OnBlockEnd` | Block execution ends | Push Kafka notification, record metrics |
| `OnTxStart` | Transaction starts | Create callTracer and prestateTracer |
| `OnTxEnd` | Transaction ends | Build Transaction, collect traces/events |
| `OnEnter` | Call frame entry | Push to call stack |
| `OnExit` | Call frame exit | Pop from stack, nest into parent |
| `OnOpcode` | Each EVM opcode | Track SSTORE, preload state |
| `OnLog` | Event emitted | Record event with trace context |
| `OnCommit` | State committed | Generate StateDiff, upload all data |

#### CallTracer

Builds a tree structure of all internal calls:

```go
type callFrame struct {
    Type              vm.OpCode       // CALL, CREATE, DELEGATECALL...
    From              common.Address
    To                *common.Address
    Input, Output     []byte
    Gas, GasUsed      uint64
    Value             *big.Int
    Error             string
    Calls             []callFrame     // Child calls (recursive)
    Logs              []Event
    ParentFailed      bool
    StorageChange     bool
    SelfStorageChange bool
}
```

**Key algorithms:**

1. **Stack management**: OnEnter pushes, OnExit pops and nests
2. **Storage tracking**: Detects SSTORE opcodes, propagates up
3. **Failure isolation**: Marks failed calls and their children
4. **ID generation**: `hash(tx_id, parent_trace_id, position)`

#### PrestateTracer

Captures state changes during transaction execution:

```go
type prestateTracer struct {
    pre     stateMap  // State before execution
    post    stateMap  // State after execution
    created map[common.Address]bool
    deleted map[common.Address]bool
}
```

**Tracking strategy:**

1. **Predictive loading**: OnOpcode preloads state for SLOAD/SSTORE/BALANCE/etc.
2. **Diff calculation**: OnTxEnd computes changed state
3. **CREATE handling**: Tracks newly created contracts

---

### Processor Module

Location: `processor/`

Handles serialization and data upload.

#### Files

| File | Purpose |
|------|---------|
| `push.go` | S3 upload, Kafka publishing, retry logic |
| `serializer.go` | Data serialization and S3 key generation |

#### PushProcessor

```go
type PushProcessor struct {
    Bucket          string
    Uploader        *s3.Client
    KafkaWriter     *kafka.Writer
    LastBlockNotice *BlockChangeNotification
    S3TempDir       string           // Local cache directory
    S3DataCh        chan *DataFile   // Async upload channel
}
```

**Features:**

- **Concurrent upload**: Uses WaitGroup for parallel S3 uploads
- **Local caching**: Optional disk cache for reliability
- **Retry logic**: Handles S3 500 errors with backoff
- **Leader-only Kafka**: Only leader publishes to Kafka

#### Serializer

Four serialization functions for different data types:

| Function | Format | S3 Key Pattern | Data |
|----------|--------|----------------|------|
| `SerializeFile` | JSON+gzip | `{chainID}/[version/]{blockID}` | BlockFile |
| `SerializeFileValidation` | JSON+gzip | `{chainID}/[version/]{height}/{blockID}` | BlockValidation |
| `SerializeHeader` | JSON+gzip | `{chainID}/[version/]{hash}/block` | Header |
| `SerializeStateDiff` | RLP | `{chainID}/[version/]{root}/stateDiff` | BlockStorageDiff |

*Note: `[version/]` is optional, included when version parameter is set during initialization.*

---

### Types Module

Location: `types/`

Core data structures for the pipeline.

> **See Also:** [Protocol Specification](protocol.md) for detailed documentation of all data types, including field descriptions, examples, and serialization formats.

#### Block-level Types

```go
// Basic block info for external consumers
type Block struct {
    ID                    string   // Block hash
    Height                *big.Int
    ParentID              string
    BaseFeePerGas         *big.Int
    Miner                 string
    GasLimit, GasUsed     *big.Int
    Timestamp             uint64
    ProcessStartTimestamp int64
}

// Complete Ethereum header
type Header struct {
    Number, Hash, ParentHash, StateRoot...
    BaseFeePerGas         // EIP-1559
    WithdrawalsRoot       // EIP-4895
    BlobGasUsed           // EIP-4844
    ParentBeaconBlockRoot // EIP-4788
}
```

#### Transaction Types

```go
type Transaction struct {
    ID               string        // Transaction hash
    From, To         string
    Gas, GasUsed     *big.Int
    GasPrice         *big.Int      // Effective gas price
    GasFeeCap        *big.Int      // EIP-1559 max fee
    GasTipCap        *big.Int      // EIP-1559 priority fee
    Input            hexutil.Bytes
    Nonce            *big.Int
    Value            *hexutil.Big
    Status           bool
    TransactionIndex int64
}
```

#### Trace Types

```go
type Trace struct {
    ID                string        // hash(tx_id, parent_id, position)
    From, To          string
    Gas, GasUsed      *big.Int
    Input, Output     hexutil.Bytes
    Value             *hexutil.Big
    CallCreateType    string        // 'create', 'suicide', 'call', 'empty'
    CallType          string        // call/delegatecall/staticcall/callcode
    TxID              string
    ParentTraceID     string
    PosInParentTrace  int64
    StorageChange     bool
    SelfStorageChange bool
    Subtraces         int64
    TraceAddress      []int64
    Error             string
}

type Event struct {
    ID            string        // hash(parent_trace_id, position)
    Address       string        // Contract address
    Selector      string        // topic[0]
    Topics        []string
    Data          hexutil.Bytes
    ParentTraceID string
    Position      int64
    LogIndex      int64
}
```

#### State Diff Types

```go
type BlockStorageDiff struct {
    Hash            common.Hash          // Current state root
    ParentHash      common.Hash          // Parent state root
    NewAccounts     []NewAccount         // Created/updated accounts
    DeletedAccounts []common.Hash        // Deleted accounts
    StorageDiff     []AccountStorageDiff // Storage changes
    NewCodes        []NewCode            // Deployed contracts
}

type NewAccount struct {
    Address  common.Hash
    Balance  *uint256.Int
    Nonce    uint64
    CodeHash common.Hash
}

type AccountStorageDiff struct {
    Address common.Hash
    Values  []IndexValuePair  // {Index, Value}
}
```

#### Aggregate Types

```go
type BlockFile struct {
    Block            Block
    Txs              []Transaction
    Events           []Event       // Successful events
    Traces           []Trace       // Successful traces
    ErrorEvents      []Event       // Failed events
    ErrorTraces      []Trace       // Failed traces
    StorageContracts []string      // Contracts with state changes
}

type BlockValidation struct {
    ValidationHash        int64  // Checksum (last 6 digits of SHA1 sum)
    IsFork                bool
    TxsCount              int
    EventsCount           int
    TracesCount           int
    ErrorEventsCount      int
    ErrorTracesCount      int
    StorageContractsCount int
}
```

---

### Leader Module

Location: `leader/`

Provides leader election for high-availability Kafka publishing.

#### Files

| File | Purpose |
|------|---------|
| `manager.go` | Leader management, mode switching |
| `leader_failover.go` | etcd-based automatic failover |
| `config.go` | Configuration types |

#### Manager

```go
type Manager struct {
    LeaderFailover *LeaderFailover
    ManualMode     bool
    IsManualBackup bool
}
```

**Two modes:**

1. **Manual mode**: Role set by `IsBackup` parameter
2. **Failover mode**: Automatic election via etcd

#### LeaderFailover

Uses etcd transactions for atomic leader election:

```go
// Try to become leader atomically
txn := client.Txn(ctx).
    If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
    Then(clientv3.OpPut(key, nodeID)).
    Else(clientv3.OpGet(key))
```

**Key features:**

- **Grace period**: Wait before assuming leadership
- **Watch-based**: Monitors key changes in real-time
- **Random backoff**: Prevents thundering herd on leader loss

---

### Writer Module

Location: `writer/`

Registers writer nodes in etcd for service discovery.

```go
type WriterNodeInfo struct {
    NodeXBucket      string
    ChainTableBucket string
    Region           string
    Brokers          []string
    Topic            string
}

type WriterRegistry struct {
    client   *clientv3.Client
    lease    clientv3.Lease
    leaseID  clientv3.LeaseID
    nodeInfo WriterNodeInfo
}
```

**Features:**

- **Lease-based registration**: Auto-cleanup on node failure
- **Conflict detection**: Prevents duplicate node IDs
- **Keep-alive**: Maintains registration with heartbeats

---

### Util Module

Location: `util/`

Common utilities for S3, Kafka, and encoding.

| File | Purpose |
|------|---------|
| `s3.go` | S3 client, upload/download with retry |
| `kafka.go` | Kafka reader/writer, message handling |
| `codec.go` | JSON+gzip and RLP encoding/decoding |
| `tracer.go` | go-ethereum type conversion |
| `to_hash.go` | MD5 hash utilities |
| `file.go` | File operations |

---

### Metrics Module

Location: `metrics/`

Observability metrics using go-ethereum metrics package.

```go
// Gauges (current values)
LatestBlockNumber         // Latest processed block
LatestBlockTime           // Latest block timestamp
LatestUploadedBlockNumber // Latest uploaded block

// Timers (performance distribution)
BlockProcessTimer         // Total block processing time
BlockTxExecutionTimer     // Transaction execution time
BlockHeaderUploadTimer    // Header upload time
StateDiffUploadTimer      // State diff upload time
BlockFileUploadTimer      // Block file upload time
BlockFileValidationTimer  // Validation upload time
BlockPushTimer            // Kafka push time
```

---

## Data Flow

Pipeline supports two integration modes with different data flows:

**Mode 1: Live Tracer**
```
Ethereum Node (block execution)
    ↓
PipelineTracer (EVM hooks)
    ↓
CallTracer + PrestateTracer (traces, events, state diff)
    ↓
Processor (serialize to JSON/gzip + RLP)
    ↓
S3 Upload (dual bucket) + Kafka Publish (BlockChangeNotification)
```

**Mode 2: RPC Tracer**
```
RPC Request (trace_debankBlock)
    ↓
Block Replay with RPCTracer
    ↓
CallTracer + PrestateTracer (traces, events, state diff)
    ↓
Return DebankOutPut (BlockFile + Header + StateDiff + ValidationHash)
```

### Block Processing Flow (Live Tracer)

```
1. OnBlockStart
   └─> Initialize BlockCtx (BlockFile, Header, BlockChange)

2. For each transaction:
   ├─> OnTxStart
   │   └─> Create callTracer + prestateTracer
   │
   ├─> During execution:
   │   ├─> OnEnter/OnExit: Build call tree
   │   ├─> OnOpcode: Track SSTORE, preload state
   │   └─> OnLog: Record events
   │
   └─> OnTxEnd
       ├─> callTracer: Flatten tree to Traces/Events
       ├─> prestateTracer: Calculate state diff
       └─> Add Transaction to BlockFile

3. OnCommit
   └─> Generate BlockStorageDiff
   └─> Concurrent upload:
       ├─> uploadBlockHeader (NodeX bucket)
       ├─> uploadBlockDiff (NodeX bucket)
       ├─> uploadBlockFile (ChainTable bucket)
       └─> uploadBlockValidation (ChainTable bucket)

4. OnBlockEnd
   └─> Push BlockChangeNotification to Kafka (leader only)
   └─> Update metrics
```

### S3 Upload Flow

```
With local cache (S3TempDir configured):
  UploadFile
    │
    ├─> Write to local file: {S3TempDir}/{bucket}/{s3key}
    ├─> Send to S3DataCh channel
    │
    └─> Background goroutine:
        ├─> Receive from S3DataCh
        ├─> Upload to S3 with retry
        └─> Delete local file on success

Without local cache:
  UploadFile
    └─> Direct S3 upload with retry
```

### Leader Election Flow

```
Failover mode:
  Start
    │
    ├─> Get(electionKey) - read current leader
    │   ├─> Key exists: Watch for changes
    │   └─> Key missing: tryToBecomeLeader
    │
    └─> Watch loop:
        ├─> PUT event:
        │   ├─> newLeader == nodeID: becomeLeader (after grace period)
        │   └─> newLeader != nodeID: loseLeadership
        │
        └─> DELETE event:
            ├─> loseLeadership (if was leader)
            └─> Random delay + tryToBecomeLeader
```

---

## Storage Strategy

### Dual Bucket Design

| Bucket | Purpose | Format | Content |
|--------|---------|--------|---------|
| NodeX (Internal) | Node synchronization | JSON+gzip, RLP | Header, StateDiff |
| ChainTable (External) | Business consumers | JSON+gzip | BlockFile, Validation |

### S3 Key Patterns

```
NodeX Bucket (without version):
  {chainID}/{blockHash}/block          # Header (JSON+gzip)
  {chainID}/{stateRoot}/stateDiff      # StateDiff (RLP)

NodeX Bucket (with version):
  {chainID}/{version}/{blockHash}/block
  {chainID}/{version}/{stateRoot}/stateDiff

ChainTable Bucket (without version):
  {chainID}/{blockID}                  # BlockFile (JSON+gzip)
  {chainID}/{blockHeight}/{blockID}    # Validation (JSON+gzip)

ChainTable Bucket (with version):
  {chainID}/{version}/{blockID}
  {chainID}/{version}/{blockHeight}/{blockID}
```

**Note:** The `version` parameter is optional. When specified during initialization, all S3 keys include a version namespace to support multiple pipeline versions or A/B testing.

### Why RLP for StateDiff?

- **Compact**: RLP is more space-efficient than JSON
- **Native**: Ethereum's standard encoding format
- **Fast**: No need for JSON parsing in internal tools

### Why JSON for BlockFile?

- **Readable**: Easy for external consumers to parse
- **Flexible**: Schema can evolve with optional fields
- **Compatible**: Works with standard tools and APIs

---

## Leader Election

### Purpose

Only one node should publish to Kafka to prevent duplicate messages and ensure consistent ordering.

### Manual Mode

```go
// Configuration
isBackup := true  // or false

// In code
if !manager.IsManualBackup {
    // This node is leader, publish to Kafka
}
```

### Failover Mode

```go
// Configuration
isBackup := nil  // Enable automatic failover

// Uses etcd key for election
// Key: {electionKey}
// Value: {nodeID}
```

**Election algorithm:**

1. On startup, try to create key with own nodeID
2. If key exists, watch for changes
3. On key deletion, wait random delay and try again
4. Grace period before assuming leadership

---

## API Reference

### Initialization

```go
// Initialize pipeline infrastructure
tracer.InitPipeline(
    region string,           // AWS region
    nodeXBucket string,      // Internal S3 bucket
    chainTableBucket string, // External S3 bucket
    brokers []string,        // Kafka broker addresses
    topic string,            // Kafka topic
    bizChainID string,       // Chain ID for S3 keys
    version string,          // Version for S3 keys (optional)
    s3TmpDir string,         // Local temp dir (optional)
)
```

### Leader Election

```go
// Setup leader election
tracer.SetupLeaderElection(
    etcdEndpoints []string,  // etcd cluster endpoints
    electionKey string,      // etcd key for election
    nodeID string,           // Unique node identifier
    version string,          // Version string
    isBackup *bool,          // nil=failover, true/false=manual
    gracePeriod time.Duration,
    writerConfig *writer.WriterNodeInfo,
)
```

### Tracer Creation

```go
// Create new tracer instance
tracer, err := tracer.NewPipelineTracer(configJSON json.RawMessage)

// Config JSON structure:
{
    "chainVariant": "ethereum"  // or "scroll", "xdc", "nitro", "l2geth"
}
```

### Tracer Hooks (for integration)

```go
// Implement these hooks in your execution client
OnBlockchainInit(chainConfig *params.ChainConfig)
OnBlockStart(event tracing.BlockEvent)
OnBlockEnd(blockErr error)
OnTxStart(vm *tracing.VMContext, tx *types.Transaction, from common.Address)
OnTxEnd(receipt *types.Receipt, err error)
OnEnter(depth int, typ byte, from, to common.Address, input []byte, gas uint64, value *big.Int)
OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool)
OnOpcode(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, ...)
OnLog(log *types.Log)
OnBalanceChange(addr common.Address, prev, new *uint256.Int, reason tracing.BalanceChangeReason)
OnCommit(originRoot, root common.Hash, destructs, accounts, storages, codes...)
OnGenesisBlock(block *types.Block, alloc types.GenesisAlloc)
OnClose()
```

---

## Deployment

### Prerequisites

- Go 1.23+
- AWS credentials with S3 access
- Kafka cluster (optional)
- etcd cluster (optional, for leader election)

### Environment Variables

```bash
# AWS credentials (or use IAM roles)
AWS_ACCESS_KEY_ID=xxx
AWS_SECRET_ACCESS_KEY=xxx
AWS_REGION=us-east-1

# etcd (if using failover mode)
ETCD_ENDPOINTS=http://localhost:2379
```

### High Availability Setup

For production deployments with high availability:

1. **Deploy multiple nodes** with the same chain configuration
2. **Configure etcd** for leader election
3. **Set gracePeriod** to allow clean handoff (e.g., 5 seconds)
4. **Monitor metrics** for failover events

```go
// Example HA configuration
tracer.SetupLeaderElection(
    []string{"etcd1:2379", "etcd2:2379", "etcd3:2379"},
    "/pipeline/leader/chain-1",
    "node-1",
    "v1",
    nil,                    // Automatic failover
    5 * time.Second,        // Grace period
    &writer.WriterNodeInfo{...},
)
```

### Monitoring

Key metrics to monitor:

| Metric | Alert Condition |
|--------|-----------------|
| `LatestBlockNumber` | Stale for > 1 minute |
| `BlockProcessTimer` | P99 > 5 seconds |
| `BlockPushTimer` | Errors increasing |
| `LatestUploadedBlockNumber` | Gap with `LatestBlockNumber` |

### Troubleshooting

**S3 upload failures:**
- Check AWS credentials and permissions
- Verify bucket exists and is accessible
- Check network connectivity

**Kafka publishing issues:**
- Verify broker connectivity
- Check topic exists
- Ensure only leader is publishing

**Leader election problems:**
- Verify etcd cluster health
- Check network connectivity to etcd
- Review grace period settings
