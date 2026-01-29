# Protocol Specification

This document provides detailed specifications for all data types used in the Pipeline system.

## Table of Contents

- [Overview](#overview)
- [Data Type Categories](#data-type-categories)
- [Block Data Types](#block-data-types)
  - [Block](#block)
  - [Header](#header)
- [Transaction Data Types](#transaction-data-types)
  - [Transaction](#transaction)
- [Trace Data Types](#trace-data-types)
  - [Trace](#trace)
  - [Event](#event)
- [State Diff Data Types](#state-diff-data-types)
  - [BlockStorageDiff](#blockstoragediff)
  - [NewAccount](#newaccount)
  - [NewCode](#newcode)
  - [AccountStorageDiff](#accountstoragediff)
  - [IndexValuePair](#indexvaluepair)
- [Aggregate Data Types](#aggregate-data-types)
  - [BlockFile](#blockfile)
  - [BlockValidation](#blockvalidation)
  - [DebankOutPut](#debankoutput)
- [Notification Data Types](#notification-data-types)
  - [BlockChangeNotification](#blockchangenotification)
  - [BlockContext](#blockcontext)
  - [OuterBlockChangeNotification](#outerblockchangenotification)
- [ID Generation](#id-generation)
- [Serialization Formats](#serialization-formats)
- [Data Relationships](#data-relationships)

---

## Overview

Pipeline uses a set of well-defined data types to represent blockchain data. These types are designed for:

- **Completeness**: Capture all relevant blockchain information
- **Efficiency**: Optimized for storage and transmission
- **Compatibility**: Easy integration with downstream systems

---

## Data Type Categories

| Category | Types | Purpose |
|----------|-------|---------|
| Block Data | Block, Header | Block metadata and full header |
| Transaction Data | Transaction | Transaction details with receipt info |
| Trace Data | Trace, Event | Internal calls and event logs |
| State Diff | BlockStorageDiff, NewAccount, etc. | State changes per block |
| Aggregate | BlockFile, BlockValidation | Combined block data for storage |
| Notification | BlockChangeNotification, etc. | Real-time block updates |

---

## Block Data Types

### Block

Simplified block metadata for external consumers.

**Source File:** `types/block.go`

**Definition:**
```go
type Block struct {
    ID                    string   `json:"id"`
    Height                *big.Int `json:"height"`
    ParentID              string   `json:"parent_id"`
    BaseFeePerGas         *big.Int `json:"base_fee_per_gas"`
    Miner                 string   `json:"miner"`
    GasLimit              *big.Int `json:"gas_limit"`
    GasUsed               *big.Int `json:"gas_used"`
    Timestamp             uint64   `json:"timestamp"`
    ProcessStartTimestamp int64    `json:"process_start_timestamp"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ID | string | `id` | Block hash (hex string with 0x prefix) |
| Height | *big.Int | `height` | Block number |
| ParentID | string | `parent_id` | Parent block hash |
| BaseFeePerGas | *big.Int | `base_fee_per_gas` | EIP-1559 base fee (nil for pre-London blocks) |
| Miner | string | `miner` | Block producer address (lowercase) |
| GasLimit | *big.Int | `gas_limit` | Maximum gas allowed in block |
| GasUsed | *big.Int | `gas_used` | Actual gas consumed |
| Timestamp | uint64 | `timestamp` | Block timestamp (Unix seconds) |
| ProcessStartTimestamp | int64 | `process_start_timestamp` | Pipeline processing start time (Unix milliseconds) |

**Example:**
```json
{
  "id": "0x1234...abcd",
  "height": 18000000,
  "parent_id": "0x5678...efgh",
  "base_fee_per_gas": 30000000000,
  "miner": "0xabcd...1234",
  "gas_limit": 30000000,
  "gas_used": 15000000,
  "timestamp": 1699900000,
  "process_start_timestamp": 1699900001234
}
```

---

### Header

Complete Ethereum block header with all fields.

**Source File:** `types/state_diff.go`

**Definition:**
```go
type Header struct {
    Number                *hexutil.Big     `json:"number"`
    Hash                  common.Hash      `json:"hash"`
    ParentHash            common.Hash      `json:"parentHash"`
    Nonce                 types.BlockNonce `json:"nonce"`
    MixHash               common.Hash      `json:"mixHash"`
    Sha3Uncles            common.Hash      `json:"sha3Uncles"`
    LogsBloom             types.Bloom      `json:"logsBloom"`
    StateRoot             common.Hash      `json:"stateRoot"`
    Miner                 common.Address   `json:"miner"`
    Difficulty            *hexutil.Big     `json:"difficulty"`
    ExtraData             hexutil.Bytes    `json:"extraData"`
    GasLimit              hexutil.Uint64   `json:"gasLimit"`
    GasUsed               hexutil.Uint64   `json:"gasUsed"`
    Timestamp             hexutil.Uint64   `json:"timestamp"`
    TransactionsRoot      common.Hash      `json:"transactionsRoot"`
    ReceiptsRoot          common.Hash      `json:"receiptsRoot"`
    BaseFeePerGas         *hexutil.Big     `json:"baseFeePerGas,omitempty"`
    WithdrawalsRoot       *common.Hash     `json:"withdrawalsRoot,omitempty"`
    BlobGasUsed           *hexutil.Uint64  `json:"blobGasUsed,omitempty"`
    ExcessBlobGas         *hexutil.Uint64  `json:"excessBlobGas,omitempty"`
    ParentBeaconBlockRoot *common.Hash     `json:"parentBeaconBlockRoot,omitempty"`
    RequestsRoot          *common.Hash     `json:"requestsRoot,omitempty"`
}
```

**Fields:**

| Field | Type | JSON Key | Description | EIP |
|-------|------|----------|-------------|-----|
| Number | *hexutil.Big | `number` | Block number | - |
| Hash | common.Hash | `hash` | Block hash | - |
| ParentHash | common.Hash | `parentHash` | Parent block hash | - |
| Nonce | types.BlockNonce | `nonce` | PoW nonce | - |
| MixHash | common.Hash | `mixHash` | PoW mix hash | - |
| Sha3Uncles | common.Hash | `sha3Uncles` | Uncles hash | - |
| LogsBloom | types.Bloom | `logsBloom` | Bloom filter for logs | - |
| StateRoot | common.Hash | `stateRoot` | State trie root | - |
| Miner | common.Address | `miner` | Block producer | - |
| Difficulty | *hexutil.Big | `difficulty` | Block difficulty | - |
| ExtraData | hexutil.Bytes | `extraData` | Extra data field | - |
| GasLimit | hexutil.Uint64 | `gasLimit` | Gas limit | - |
| GasUsed | hexutil.Uint64 | `gasUsed` | Gas used | - |
| Timestamp | hexutil.Uint64 | `timestamp` | Block timestamp | - |
| TransactionsRoot | common.Hash | `transactionsRoot` | Transactions trie root | - |
| ReceiptsRoot | common.Hash | `receiptsRoot` | Receipts trie root | - |
| BaseFeePerGas | *hexutil.Big | `baseFeePerGas` | Base fee per gas | EIP-1559 |
| WithdrawalsRoot | *common.Hash | `withdrawalsRoot` | Withdrawals trie root | EIP-4895 |
| BlobGasUsed | *hexutil.Uint64 | `blobGasUsed` | Blob gas used | EIP-4844 |
| ExcessBlobGas | *hexutil.Uint64 | `excessBlobGas` | Excess blob gas | EIP-4844 |
| ParentBeaconBlockRoot | *common.Hash | `parentBeaconBlockRoot` | Parent beacon block root | EIP-4788 |
| RequestsRoot | *common.Hash | `requestsRoot` | Requests trie root | EIP-7685 |

---

## Transaction Data Types

### Transaction

Transaction with merged receipt information.

**Source File:** `types/transaction.go`

**Definition:**
```go
type Transaction struct {
    ID               string        `json:"id"`
    From             string        `json:"from_addr"`
    To               string        `json:"to_addr"`
    Gas              *big.Int      `json:"gas_limit"`
    GasPrice         *big.Int      `json:"gas_price"`
    GasUsed          *big.Int      `json:"gas_used"`
    Status           bool          `json:"status"`
    GasFeeCap        *big.Int      `json:"max_fee_per_gas"`
    GasTipCap        *big.Int      `json:"max_priority_fee_per_gas"`
    Input            hexutil.Bytes `json:"input"`
    Nonce            *big.Int      `json:"nonce"`
    TransactionIndex int64         `json:"idx"`
    Value            *hexutil.Big  `json:"value"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ID | string | `id` | Transaction hash |
| From | string | `from_addr` | Sender address |
| To | string | `to_addr` | Recipient address (contract address for CREATE) |
| Gas | *big.Int | `gas_limit` | Gas limit |
| GasPrice | *big.Int | `gas_price` | Effective gas price (from receipt) |
| GasUsed | *big.Int | `gas_used` | Actual gas consumed |
| Status | bool | `status` | Execution status (true=success, false=revert) |
| GasFeeCap | *big.Int | `max_fee_per_gas` | EIP-1559 max fee per gas |
| GasTipCap | *big.Int | `max_priority_fee_per_gas` | EIP-1559 max priority fee |
| Input | hexutil.Bytes | `input` | Transaction input data |
| Nonce | *big.Int | `nonce` | Transaction nonce |
| TransactionIndex | int64 | `idx` | Position in block |
| Value | *hexutil.Big | `value` | Transfer value in wei |

**Notes:**
- `To` field contains the **created contract address** for contract creation transactions
- `GasPrice` is the **effective gas price** from the receipt, not the transaction's original gas price
- `Status` is derived from `receipt.Status == 1`

**Example:**
```json
{
  "id": "0xabc123...",
  "from_addr": "0x1234...5678",
  "to_addr": "0xabcd...efgh",
  "gas_limit": 21000,
  "gas_price": 30000000000,
  "gas_used": 21000,
  "status": true,
  "max_fee_per_gas": 50000000000,
  "max_priority_fee_per_gas": 2000000000,
  "input": "0x",
  "nonce": 42,
  "idx": 0,
  "value": "0xde0b6b3a7640000"
}
```

---

## Trace Data Types

### Trace

Internal call trace representing a single call frame.

**Source File:** `types/trace.go`

**Definition:**
```go
type Trace struct {
    ID                string        `json:"id"`
    From              string        `json:"from_addr"`
    Gas               *big.Int      `json:"gas_limit"`
    Input             hexutil.Bytes `json:"input"`
    To                string        `json:"to_addr"`
    Value             *hexutil.Big  `json:"value"`
    GasUsed           *big.Int      `json:"gas_used"`
    Output            hexutil.Bytes `json:"output"`
    CallCreateType    string        `json:"type"`
    CallType          string        `json:"call_type"`
    TxID              string        `json:"tx_id"`
    ParentTraceID     string        `json:"parent_trace_id"`
    PosInParentTrace  int64         `json:"pos_in_parent_trace"`
    SelfStorageChange bool          `json:"self_storage_change"`
    StorageChange     bool          `json:"storage_change"`
    Subtraces         int64         `json:"subtraces"`
    TraceAddress      []int64       `json:"trace_address"`
    Error             string        `json:"error,omitempty"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ID | string | `id` | Trace ID (see [ID Generation](#id-generation)) |
| From | string | `from_addr` | Caller address |
| Gas | *big.Int | `gas_limit` | Gas allocated to this call |
| Input | hexutil.Bytes | `input` | Call input data |
| To | string | `to_addr` | Target address |
| Value | *hexutil.Big | `value` | Value transferred |
| GasUsed | *big.Int | `gas_used` | Gas actually used |
| Output | hexutil.Bytes | `output` | Return data |
| CallCreateType | string | `type` | High-level type: `create`, `suicide`, `call`, `empty` |
| CallType | string | `call_type` | EVM call type: `call`, `delegatecall`, `staticcall`, `callcode` |
| TxID | string | `tx_id` | Parent transaction hash |
| ParentTraceID | string | `parent_trace_id` | Parent trace ID (empty for root trace) |
| PosInParentTrace | int64 | `pos_in_parent_trace` | Position within parent trace |
| SelfStorageChange | bool | `self_storage_change` | Whether this call executed SSTORE |
| StorageChange | bool | `storage_change` | Whether this call or children modified storage |
| Subtraces | int64 | `subtraces` | Number of direct child calls |
| TraceAddress | []int64 | `trace_address` | Path from root (e.g., [0, 1, 0]) |
| Error | string | `error` | Error message if call failed |

**CallCreateType Values:**

| Value | Description |
|-------|-------------|
| `create` | Contract creation (CREATE/CREATE2) |
| `suicide` | Self-destruct (SELFDESTRUCT) |
| `call` | Normal call (CALL/DELEGATECALL/STATICCALL/CALLCODE) |
| `empty` | Empty/no-op trace |

**CallType Values:**

| Value | Description |
|-------|-------------|
| `call` | CALL opcode |
| `delegatecall` | DELEGATECALL opcode |
| `staticcall` | STATICCALL opcode |
| `callcode` | CALLCODE opcode |
| `create` | CREATE opcode |
| `create2` | CREATE2 opcode |

**Example:**
```json
{
  "id": "0xdef456...",
  "from_addr": "0x1234...5678",
  "gas_limit": 100000,
  "input": "0xa9059cbb...",
  "to_addr": "0xtoken...addr",
  "value": "0x0",
  "gas_used": 50000,
  "output": "0x0000...0001",
  "type": "call",
  "call_type": "call",
  "tx_id": "0xabc123...",
  "parent_trace_id": "",
  "pos_in_parent_trace": 0,
  "self_storage_change": true,
  "storage_change": true,
  "subtraces": 2,
  "trace_address": []
}
```

---

### Event

Event log emitted during execution.

**Source File:** `types/event.go`

**Definition:**
```go
type Event struct {
    ID            string        `json:"id"`
    Address       string        `json:"contract_id"`
    Selector      string        `json:"selector"`
    Topics        []string      `json:"topics"`
    Data          hexutil.Bytes `json:"data"`
    ParentTraceID string        `json:"parent_trace_id"`
    Position      int64         `json:"pos_in_parent_trace"`
    LogIndex      int64         `json:"idx"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ID | string | `id` | Event ID (see [ID Generation](#id-generation)) |
| Address | string | `contract_id` | Contract that emitted the event |
| Selector | string | `selector` | Event signature (topic[0]) |
| Topics | []string | `topics` | Indexed parameters (including selector) |
| Data | hexutil.Bytes | `data` | Non-indexed parameters |
| ParentTraceID | string | `parent_trace_id` | Trace that emitted this event |
| Position | int64 | `pos_in_parent_trace` | Position within parent trace |
| LogIndex | int64 | `idx` | Global log index in block (0 for failed events) |

**Notes:**
- `LogIndex` is set to **0** for events in failed calls (stored in `ErrorEvents`)
- `Selector` is the first topic (event signature hash)

**Example:**
```json
{
  "id": "0x789abc...",
  "contract_id": "0xtoken...addr",
  "selector": "0xddf252ad...",
  "topics": [
    "0xddf252ad...",
    "0x000...sender",
    "0x000...receiver"
  ],
  "data": "0x000...amount",
  "parent_trace_id": "0xdef456...",
  "pos_in_parent_trace": 0,
  "idx": 5
}
```

---

## State Diff Data Types

### BlockStorageDiff

Complete state changes for a block.

**Source File:** `types/state_diff.go`

**Serialization:** RLP encoded

**Definition:**
```go
type BlockStorageDiff struct {
    Hash            common.Hash
    ParentHash      common.Hash
    NewAccounts     []NewAccount
    DeletedAccounts []common.Hash
    StorageDiff     []AccountStorageDiff
    NewCodes        []NewCode
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| Hash | common.Hash | State root after block execution |
| ParentHash | common.Hash | State root before block execution |
| NewAccounts | []NewAccount | Created or updated accounts |
| DeletedAccounts | []common.Hash | Deleted accounts (SELFDESTRUCT) |
| StorageDiff | []AccountStorageDiff | Storage slot changes |
| NewCodes | []NewCode | Newly deployed contract codes |

---

### NewAccount

Account state change.

**Definition:**
```go
type NewAccount struct {
    Address  common.Hash
    Balance  *uint256.Int
    Nonce    uint64
    CodeHash common.Hash
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| Address | common.Hash | Account address (as 32-byte hash) |
| Balance | *uint256.Int | New balance (nil if unchanged) |
| Nonce | uint64 | New nonce (0 if unchanged) |
| CodeHash | common.Hash | Code hash (empty if unchanged) |

**Note:** Address is stored as `keccak256(address)` for consistent 32-byte representation.

---

### NewCode

Newly deployed contract code.

**Definition:**
```go
type NewCode struct {
    CodeHash common.Hash
    Code     []byte
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| CodeHash | common.Hash | Keccak256 hash of code |
| Code | []byte | Full contract bytecode |

---

### AccountStorageDiff

Storage changes for a single account.

**Definition:**
```go
type AccountStorageDiff struct {
    Address common.Hash
    Values  []IndexValuePair
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| Address | common.Hash | Account address (as 32-byte hash) |
| Values | []IndexValuePair | Changed storage slots |

---

### IndexValuePair

Single storage slot change.

**Definition:**
```go
type IndexValuePair struct {
    Index common.Hash
    Value *uint256.Int
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| Index | common.Hash | Storage slot key (`keccak256(slot)`) |
| Value | *uint256.Int | New value |

---

## Aggregate Data Types

### BlockFile

Complete block data for external storage.

**Source File:** `types/block_file.go`

**Serialization:** JSON + gzip

**Definition:**
```go
type BlockFile struct {
    Block            Block         `json:"block"`
    Txs              []Transaction `json:"txs"`
    Events           []Event       `json:"events"`
    Traces           []Trace       `json:"traces"`
    ErrorEvents      []Event       `json:"error_events"`
    ErrorTraces      []Trace       `json:"error_traces"`
    StorageContracts []string      `json:"storage_contracts"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| Block | Block | `block` | Block metadata |
| Txs | []Transaction | `txs` | All transactions |
| Events | []Event | `events` | Successful events |
| Traces | []Trace | `traces` | Successful traces |
| ErrorEvents | []Event | `error_events` | Events from failed calls |
| ErrorTraces | []Trace | `error_traces` | Failed call traces |
| StorageContracts | []string | `storage_contracts` | Addresses with storage changes |

**Notes:**
- `Events` and `Traces` contain only data from **successful** calls
- `ErrorEvents` and `ErrorTraces` contain data from **failed** calls (reverted or errored)
- `StorageContracts` lists all contract addresses that had SSTORE operations

---

### BlockValidation

Block integrity validation data.

**Source File:** `types/block_file.go`

**Definition:**
```go
type BlockValidation struct {
    ValidationHash        int64 `json:"validation_hash"`
    IsFork                bool  `json:"is_fork"`
    TxsCount              int   `json:"txs_count"`
    EventsCount           int   `json:"events_count"`
    TracesCount           int   `json:"traces_count"`
    ErrorEventsCount      int   `json:"error_events_count"`
    ErrorTracesCount      int   `json:"error_traces_count"`
    StorageContractsCount int   `json:"storage_contracts_count"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ValidationHash | int64 | `validation_hash` | Checksum of all IDs (last 6 digits) |
| IsFork | bool | `is_fork` | Whether block is on a fork |
| TxsCount | int | `txs_count` | Number of transactions |
| EventsCount | int | `events_count` | Number of successful events |
| TracesCount | int | `traces_count` | Number of successful traces |
| ErrorEventsCount | int | `error_events_count` | Number of failed events |
| ErrorTracesCount | int | `error_traces_count` | Number of failed traces |
| StorageContractsCount | int | `storage_contracts_count` | Number of contracts with storage changes |

**ValidationHash Calculation:**
```go
func CalcValidationHash(ids []string) int64 {
    sha1Sum := big.NewInt(0)
    for _, id := range ids {
        hash := sha1(id)
        sha1Sum.Add(sha1Sum, hash.ToBigInt())
    }
    return last6Digits(sha1Sum)
}
```

The validation hash is computed from:
1. Block ID
2. All Transaction IDs
3. All Event IDs (successful only)
4. All Trace IDs (successful only)

---

### DebankOutPut

Complete output for RPC tracer.

**Source File:** `types/output.go`

**Definition:**
```go
type DebankOutPut struct {
    BlockFile      *BlockFile    `json:"block_file"`
    Header         *Header       `json:"header"`
    StateDiff      hexutil.Bytes `json:"state_diff"`
    ValidationHash int64         `json:"validation_hash"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| BlockFile | *BlockFile | `block_file` | Complete block data |
| Header | *Header | `header` | Full block header |
| StateDiff | hexutil.Bytes | `state_diff` | RLP-encoded BlockStorageDiff |
| ValidationHash | int64 | `validation_hash` | Validation checksum |

---

## Notification Data Types

### BlockChangeNotification

Real-time block change notification for Kafka.

**Source File:** `types/block_notification.go`

**Serialization:** JSON + gzip

**Definition:**
```go
type BlockChangeNotification struct {
    ChangeType uint64         `json:"changeType"`
    NewBlocks  []BlockContext `json:"newBlocks"`
    DropBlocks []BlockContext `json:"dropBlocks"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ChangeType | uint64 | `changeType` | 1=new block, 2=fork/reorg |
| NewBlocks | []BlockContext | `newBlocks` | New blocks (sorted by height) |
| DropBlocks | []BlockContext | `dropBlocks` | Dropped blocks due to fork |

**ChangeType Values:**

| Value | Description |
|-------|-------------|
| 1 | New block added to canonical chain |
| 2 | Chain reorganization (fork) |

---

### BlockContext

Minimal block information for notifications.

**Definition:**
```go
type BlockContext struct {
    Hash        common.Hash `json:"hash"`
    ParentHash  common.Hash `json:"parentHash"`
    BlockNumber uint64      `json:"blockNumber"`
    Timestamp   uint64      `json:"timestamp"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| Hash | common.Hash | `hash` | Block hash |
| ParentHash | common.Hash | `parentHash` | Parent block hash |
| BlockNumber | uint64 | `blockNumber` | Block height |
| Timestamp | uint64 | `timestamp` | Block timestamp |

---

### OuterBlockChangeNotification

Simplified notification for external systems.

**Definition:**
```go
type OuterBlockChangeNotification struct {
    ChainID     int64       `json:"chain_id"`
    Hash        common.Hash `json:"block_id"`
    BlockNumber uint64      `json:"block_height"`
    Timestamp   uint64      `json:"block_timestamp"`
    IsFork      bool        `json:"is_fork"`
}
```

**Fields:**

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| ChainID | int64 | `chain_id` | Chain identifier |
| Hash | common.Hash | `block_id` | Block hash |
| BlockNumber | uint64 | `block_height` | Block height |
| Timestamp | uint64 | `block_timestamp` | Block timestamp |
| IsFork | bool | `is_fork` | Whether this is a fork notification |

---

## ID Generation

### Trace ID

```
TraceID = MD5(TxID + ParentTraceID + PosInParentTrace)
```

- For root trace: `ParentTraceID = ""`
- `PosInParentTrace` is the position within the parent call's child calls

### Event ID

```
EventID = MD5(ParentTraceID + PosInParentTrace)
```

- `ParentTraceID` is the trace that emitted the event
- `PosInParentTrace` is the event's position within that trace

### Implementation

```go
func ToHash(args []string) string {
    hasher := md5.New()
    for _, arg := range args {
        hasher.Write([]byte(arg))
    }
    return hex.EncodeToString(hasher.Sum(nil))
}

// Trace ID
traceID := ToHash([]string{txID, parentTraceID, fmt.Sprintf("%d", pos)})

// Event ID
eventID := ToHash([]string{parentTraceID, fmt.Sprintf("%d", pos)})
```

---

## Serialization Formats

| Data Type | Format | Compression | Storage |
|-----------|--------|-------------|---------|
| BlockFile | JSON | gzip | ChainTable S3 |
| BlockValidation | JSON | gzip | ChainTable S3 |
| Header | JSON | gzip | NodeX S3 |
| BlockStorageDiff | RLP | none | NodeX S3 |
| BlockChangeNotification | JSON | gzip | Kafka |

---

## Data Relationships

```
BlockFile
├── Block (1)
├── Txs (N)
│   └── Transaction
├── Traces (N) ─────────────────┐
│   └── Trace                   │
│       ├── ParentTraceID ──────┤ (self-referential)
│       └── TxID ───────────────┼── Transaction.ID
├── Events (N)                  │
│   └── Event                   │
│       └── ParentTraceID ──────┘
├── ErrorTraces (N)
│   └── Trace (same structure)
└── ErrorEvents (N)
    └── Event (same structure)

BlockStorageDiff
├── NewAccounts (N)
│   └── NewAccount
├── DeletedAccounts (N)
│   └── common.Hash (address)
├── StorageDiff (N)
│   └── AccountStorageDiff
│       └── Values (N)
│           └── IndexValuePair
└── NewCodes (N)
    └── NewCode
```

**Trace Tree Structure:**
```
Transaction
└── Root Trace (trace_address: [])
    ├── Child Trace 1 (trace_address: [0])
    │   ├── Grandchild 1 (trace_address: [0, 0])
    │   └── Grandchild 2 (trace_address: [0, 1])
    └── Child Trace 2 (trace_address: [1])
        └── Grandchild 3 (trace_address: [1, 0])
```
