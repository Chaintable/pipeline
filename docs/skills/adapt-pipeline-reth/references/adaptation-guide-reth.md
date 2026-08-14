# Adapting Reth for Pipeline Integration

This guide is based on the adaptation from `v1.10.0-rc.2` to `v1.10.0-rc.2-debank-2` in the reth-x project. It describes how to integrate Pipeline's `trace_debankBlock` RPC method into reth (Rust Ethereum client).

> **How this differs from the go-ethereum guides:**
>
> | Dimension | go-ethereum (Standard) | go-ethereum (Legacy) | Reth |
> |-----------|----------------------|----------------------|------|
> | Language | Go | Go | Rust |
> | Integration mode | Live Tracer + RPC Tracer | Live Tracer (EVMLogger) | **RPC Tracer only** |
> | Pipeline code | External Go module | Embedded Go source | **Reimplemented in Rust** |
> | StateDiff strategy | StateDB-based (OnCommit) | StateDB-based (OnCommit) | **CacheDB-based (dual DB)** |
> | Tracing framework | `tracing.Hooks` | `vm.EVMLogger` | `revm-inspectors` (`TracingInspector`) |
> | S3/Kafka upload | Built-in | Built-in | **Not included** (RPC output only) |
> | Block lifecycle hooks | OnBlockStart/OnBlockEnd | Manual dispatch | **Not needed** (on-demand replay) |
> | No core EVM modification | Hooks + StateDB changes | EVMLogger + StateDB changes | **No EVM/StateDB changes** |
>
> **Key insight:** Reth's adaptation is purely an RPC extension - it does NOT modify the core EVM, StateDB, or block processing pipeline. All Pipeline data types and trace logic are reimplemented in Rust within the RPC layer, and tracing is done on-demand via block replay using reth's existing `revm-inspectors` framework.

---

## Adaptation Overview

| Crate / File | Change Type | Description |
|-------------|-------------|-------------|
| `crates/rpc/rpc-eth-types/src/debank.rs` | New file | Core types (BlockFile, Trace, Event, StateDiff) and conversion logic |
| `crates/rpc/rpc-eth-types/src/lib.rs` | Modify | Export debank module |
| `crates/rpc/rpc-eth-types/src/cache/db.rs` | Modify | Add `StateDiffTraceDB` wrapper |
| `crates/rpc/rpc-eth-types/src/receipt.rs` | Modify | Add default `get_deposit_nonce` |
| `crates/rpc/rpc-eth-types/Cargo.toml` | Modify | Add dependencies (md-5, sha1, alloy-rlp, etc.) |
| `crates/rpc/rpc-eth-api/src/helpers/trace.rs` | Modify | Add `trace_all_block` method |
| `crates/rpc/rpc-api/src/trace.rs` | Modify | Add `trace_debankBlock` RPC definition |
| `crates/rpc/rpc/src/trace.rs` | Modify | Implement `trace_debank_block` |
| `crates/rpc/rpc/Cargo.toml` | Modify | Add `reth-rpc-eth-types` dependency |
| `crates/rpc/rpc-convert/src/transaction.rs` | Modify | Add `get_deposit_nonce` trait method |
| `crates/rpc/rpc-builder/src/lib.rs` | Modify | Update trait bounds for `register_trace` |
| `crates/optimism/rpc/src/eth/receipt.rs` | Modify | Implement OP-specific `get_deposit_nonce` |
| `Cargo.toml` | Modify | Add workspace dependencies |

---

## Step 1: Reimplement Pipeline Types in Rust (crates/rpc/rpc-eth-types/src/debank.rs)

This is the core of the adaptation. All Pipeline data types from the Go `types/` package are reimplemented in Rust as a new module `debank.rs`.

### 1.1 Data Types

The following types mirror their Go counterparts:

| Rust Type | Go Equivalent | Serialization |
|-----------|---------------|---------------|
| `BlockStorageDiff` | `types.BlockStorageDiff` | RLP (via `alloy-rlp` derive) |
| `NewAccount` | `types.NewAccount` | RLP |
| `NewCode` | `types.NewCode` | RLP |
| `AccountStorageDiff` | `types.AccountStorageDiff` | RLP |
| `IndexValuePair` | `types.IndexValuePair` | RLP |
| `DebankBlock` | `types.Block` | JSON |
| `DebankTransaction` | `types.Transaction` | JSON |
| `DebankTrace` | `types.Trace` | JSON |
| `DebankEvent` | `types.Event` | JSON |
| `BlockFile` | `types.BlockFile` | JSON |
| `BlockValidation` | `types.BlockValidation` | JSON |
| `DebankOutPut` | `types.DebankOutPut` | JSON |

### 1.2 Key Conversions

**From `revm-inspectors` trace nodes to Debank types:**

```rust
impl From<&CallTraceNode> for DebankTrace { ... }
impl From<&CallLog> for DebankEvent { ... }
```

The conversion from `CallTraceNode` handles:
- Mapping `CallKind` (Call/StaticCall/DelegateCall/Create/Create2) to Debank's `call_create_type` and `call_type`
- Detecting `SSTORE` opcodes in trace steps for `storage_change` flags
- Error message formatting matching go-ethereum's error strings

**Trace tree building:**

```rust
pub fn build_debank_traces(
    tx_id: H256,
    traces: CallTraceArena,
    log_index: &RefCell<usize>,
) -> (Vec<DebankTrace>, Vec<DebankTrace>, Vec<DebankEvent>, Vec<DebankEvent>)
```

This function recursively walks the `CallTraceArena` tree and:
1. Assigns Debank IDs using MD5 hash (matching Go's `util.ToHash`)
2. Builds `trace_address` paths
3. Separates successful vs error traces/events
4. Propagates `storage_change` flags up the call tree
5. Handles selfdestruct as additional child traces
6. Tracks global `log_index` across transactions

**StateDiff from CacheDB:**

```rust
pub fn get_storage_diffs_from_cache<DB: DatabaseRef>(cache: Cache, pre_db: DB) -> BlockStorageDiff
```

Instead of the Go approach (hooking into StateDB's `OnCommit`), reth uses a dual-database approach:
- `pre_db`: Read-only database with state before block execution
- `diff.cache`: In-memory database that captures all state changes during replay
- After replay, compare `cache` against `pre_db` to compute the diff

### 1.3 ID Generation

IDs must be compatible with the Go implementation:

```rust
pub trait DebankID {
    fn calculate_id(args: Vec<&str>) -> String {
        let mut hasher = Md5::new();
        for arg in args {
            hasher.update(arg.as_bytes());
        }
        format!("{:x}", hasher.finalize())
    }
}
```

Validation hash uses SHA1 (matching Go's `CalcValidationHash`):

```rust
pub fn calc_validation_hash(ids: &[String]) -> i64 {
    let mut sha1_sum = U256::from(0);
    for each in ids {
        let mut hasher = Sha1::new();
        hasher.update(each.as_bytes());
        sha1_sum += U256::from_str_radix(&hex::encode(hasher.finalize()), 16).unwrap();
    }
    // Take last 6 digits
    let s = sha1_sum.to_string();
    i64::from_str(&s[s.len().saturating_sub(6)..]).unwrap_or(0)
}
```

### 1.4 Genesis Block Handling

```rust
pub fn build_genesis_txs_and_traces(genesis: &Genesis) -> (Vec<DebankTransaction>, Vec<DebankTrace>)
```

Matches the Go implementation's genesis handling:
- Sorted addresses for deterministic output
- `0xgenesis01...` transfer txs for accounts with balance
- `0xgenesis02...` create txs for accounts with code
- `0xgenesis03...` native token contract creation (0xeeee...eeee)

### 1.5 Dependencies

Add to `crates/rpc/rpc-eth-types/Cargo.toml`:

```toml
md-5.workspace = true
sha1.workspace = true
alloy-rlp.workspace = true
alloy-genesis.workspace = true
revm-bytecode.workspace = true
```

---

## Step 2: Add StateDiffTraceDB (crates/rpc/rpc-eth-types/src/cache/db.rs)

A wrapper database that captures state changes for diff computation:

```rust
pub struct StateDiffTraceDB<ExtDB> {
    /// In-memory database that stores all state changes
    pub diff: InMemoryDB,
    /// The underlying database (read-write, for actual execution)
    pub db: ExtDB,
}
```

Key design:
- `Database` trait delegates reads to `db` (the real state)
- `DatabaseCommit` writes to **both** `diff` and `db`
- After execution, `diff.cache` contains all changed accounts/storage
- Compare `diff.cache` against a separate `pre_db` to compute the actual diff

This is fundamentally different from the Go approach where `OnCommit` provides the diff directly from StateDB internals.

---

## Step 3: Add trace_all_block Helper (crates/rpc/rpc-eth-api/src/helpers/trace.rs)

Add a new method to the `Trace` trait that replays all transactions and collects both traces and state diff:

```rust
fn trace_all_block<Setup, Insp, F, R>(
    &self,
    block_id: BlockId,
    inspector_setup: Setup,
    f: F,
) -> impl Future<Output = Result<(Vec<R>, BlockStorageDiff, Vec<Address>), Self::Error>>
```

This method:
1. Creates two state providers from the parent block state
2. Wraps one in `StateDiffTraceDB` for execution (captures changes)
3. Keeps the other as `pre_db` for comparison
4. Replays all transactions with the tracing inspector
5. After execution, extracts storage diffs and changed contract addresses from the diff cache

---

## Step 4: Define the RPC Method (crates/rpc/rpc-api/src/trace.rs)

Add the `trace_debankBlock` method to the `TraceApi` trait:

```rust
/// Returns debank's trace information for a given block.
#[method(name = "debankBlock")]
async fn trace_debank_block(&self, block_id: BlockId) -> RpcResult<DebankOutPut>;
```

---

## Step 5: Implement trace_debank_block (crates/rpc/rpc/src/trace.rs)

The main implementation:

```rust
pub async fn trace_debank_block(&self, block_id: BlockId) -> Result<DebankOutPut, Eth::Error> {
    // 1. Fetch the block and build DebankBlock + header
    // 2. If genesis block (number == 0):
    //    - Build state diff from genesis alloc
    //    - Build genesis txs and traces
    //    - Return immediately
    // 3. Fetch receipts and build DebankTransactions
    // 4. Check for empty blocks (parent_root == block_root) - return early with empty diff
    // 5. Replay block with TracingInspector configured for:
    //    - Parity-style tracing
    //    - Step recording (for SSTORE detection)
    //    - Log recording
    //    - OpcodeFilter for SSTORE only
    // 6. For each tx, call build_debank_traces to convert traces
    // 7. Collect state diff from StateDiffTraceDB
    // 8. Set state roots and return DebankOutPut
}
```

The `TracingInspector` configuration:

```rust
let mut trace_cfg = TracingInspectorConfig::default_parity()
    .set_steps(true)           // Record opcode steps (for SSTORE detection)
    .set_record_logs(true)     // Record event logs
    .set_exclude_precompile_calls(false);
trace_cfg.record_opcodes_filter = Some(OpcodeFilter::new().enabled(OpCode::SSTORE));
```

---

## Step 6: Update Trait Bounds (crates/rpc/rpc-builder/src/lib.rs)

The `register_trace` method needs updated bounds since `trace_debank_block` requires block and receipt access:

```rust
pub fn register_trace(&mut self) -> &mut Self
where
    EthApi: TraceExt + EthBlocks + LoadReceipt,  // Added EthBlocks + LoadReceipt
```

---

## Step 7: Add deposit_nonce Support (crates/rpc/rpc-convert/src/transaction.rs)

Add a `get_deposit_nonce` method to the receipt converter traits for L2 chains:

```rust
// In ReceiptConverter trait:
fn get_deposit_nonce(&self, receipt_response: &Self::RpcReceipt) -> Option<u64>;

// In RpcConvert trait (default impl):
fn get_deposit_nonce(&self, _receipt: &RpcReceipt<Self::Network>) -> Option<u64> {
    None  // Non-deposit chains return None
}
```

For Optimism (`crates/optimism/rpc/src/eth/receipt.rs`):

```rust
fn get_deposit_nonce(&self, receipt_response: &Self::RpcReceipt) -> Option<u64> {
    match &receipt_response.inner.inner.receipt {
        OpReceipt::Deposit(deposit_receipt) => deposit_receipt.deposit_nonce,
        _ => None,
    }
}
```

---

## Adaptation Checklist

### Required (RPC Tracer)

- [ ] `crates/rpc/rpc-eth-types/src/debank.rs` - Implement all Pipeline types in Rust
- [ ] `crates/rpc/rpc-eth-types/src/lib.rs` - Export debank module
- [ ] `crates/rpc/rpc-eth-types/src/cache/db.rs` - Add `StateDiffTraceDB`
- [ ] `crates/rpc/rpc-eth-types/Cargo.toml` - Add dependencies (md-5, sha1, alloy-rlp, alloy-genesis, revm-bytecode)
- [ ] `crates/rpc/rpc-eth-api/src/helpers/trace.rs` - Add `trace_all_block` method
- [ ] `crates/rpc/rpc-api/src/trace.rs` - Add `trace_debankBlock` RPC definition
- [ ] `crates/rpc/rpc/src/trace.rs` - Implement `trace_debank_block`
- [ ] `crates/rpc/rpc/Cargo.toml` - Add `reth-rpc-eth-types` dependency
- [ ] `crates/rpc/rpc-builder/src/lib.rs` - Update trait bounds
- [ ] `Cargo.toml` - Add workspace dependencies

### L2-Specific (Optimism)

- [ ] `crates/rpc/rpc-convert/src/transaction.rs` - Add `get_deposit_nonce` to traits
- [ ] `crates/rpc/rpc-eth-types/src/receipt.rs` - Default `get_deposit_nonce` returning None
- [ ] `crates/optimism/rpc/src/eth/receipt.rs` - Implement OP-specific `get_deposit_nonce`

### NOT Required (compared to go-ethereum)

- No `core/tracing/hooks.go` equivalent (uses `revm-inspectors`)
- No `core/state/statedb.go` modification (uses `StateDiffTraceDB` wrapper)
- No `core/state_processor.go` modification (reth replays blocks via existing infra)
- No `core/blockchain.go` modification (no live tracer, no Kafka push)
- No embedded pipeline source code (types reimplemented in Rust)
- No S3/Kafka integration (RPC-only output)
- No leader election
- No CLI flags for tracer configuration

---

## Architecture Comparison

```
go-ethereum (Live Tracer)              Reth (RPC Tracer only)
─────────────────────────              ──────────────────────────
Block execution hooks              ──►  On-demand block replay
OnCommit for state diff            ──►  Dual-DB (StateDiffTraceDB) comparison
PipelineTracer implements hooks    ──►  TracingInspector (revm-inspectors)
callTracer (Go)                    ──►  CallTraceArena (Rust)
prestateTracer (Go)                ──►  StateDiffTraceDB cache diff
S3 upload + Kafka push             ──►  RPC response only
pipeline/ Go module                ──►  debank.rs Rust module
util.ToHash (MD5)                  ──►  md5 crate
types.CalcValidationHash (SHA1)    ──►  sha1 crate
RLP via go-ethereum/rlp            ──►  alloy-rlp derive macros
JSON via encoding/json             ──►  serde + serde_json
```

---

## StateDiff Strategy: Dual-DB vs OnCommit

The most significant architectural difference is how state diffs are computed:

**go-ethereum (OnCommit hook):**
```
StateDB.commitAndFlush()
    → Computes state update internally
    → Calls OnCommit(originRoot, root, destructs, accounts, storages, codes)
    → Pipeline directly receives structured diff data
```

**Reth (Dual-DB comparison):**
```
Create pre_db (snapshot of parent state)
Create StateDiffTraceDB { diff: InMemoryDB, db: StateCacheDb }
    → Execute all transactions against StateDiffTraceDB
    → db handles real execution, diff captures all commits
After execution:
    → Compare diff.cache against pre_db
    → Compute BlockStorageDiff from differences
```

The dual-DB approach is necessary because reth does not expose commit-level hooks. It achieves the same result without modifying the core EVM or state database.
