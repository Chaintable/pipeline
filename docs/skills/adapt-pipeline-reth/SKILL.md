---
name: adapt-pipeline-reth
description: "Adapt Reth (Rust Ethereum client) to integrate Pipeline RPC Tracer (trace_debankBlock). No core EVM changes needed."
user-invocable: true
argument-hint: "<path-to-reth-fork>"
---

# Reth Pipeline Adaptation (7 Phases)

You are adapting a **Reth (Rust Ethereum)** client to integrate Pipeline's `trace_debankBlock` RPC method. Unlike go-ethereum adaptations, Reth uses **RPC Tracer only** — no core EVM or StateDB modifications are needed. All Pipeline types are reimplemented in Rust.

**Target repository**: `$ARGUMENTS`
**Reference document**: Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` for detailed code examples.
**Pipeline repo**: The current working directory contains the Pipeline source code.

## Important Principles

1. **Explore before modifying** — Read target files to understand existing patterns
2. **Incremental verification** — Run `cargo build` (or `cargo check`) after each phase
3. **Rust idioms** — Use derive macros (`RlpEncodable`, `Serialize`), proper error handling, trait bounds
4. **No EVM changes** — Everything happens in the RPC layer
5. **Compatibility** — ID generation (MD5) and validation hash (SHA1) must match the Go implementation exactly

## Architecture Overview

```
Reth adaptation is purely an RPC extension:
- No core/tracing/ changes (uses revm-inspectors)
- No core/state/ changes (uses StateDiffTraceDB wrapper)
- No core/blockchain/ changes (on-demand replay, no live tracing)
- No S3/Kafka (RPC output only)
```

---

## Phase 1: Implement Pipeline Types in Rust

### Goal
Create `debank.rs` with all Pipeline data types reimplemented in Rust.

### Explore
1. Read `$ARGUMENTS/crates/rpc/rpc-eth-types/src/lib.rs` — understand module structure
2. Read `$ARGUMENTS/crates/rpc/rpc-eth-types/Cargo.toml` — check existing dependencies
3. Read Pipeline Go types for reference:
   - `types/block.go`, `types/transaction.go`, `types/trace.go`, `types/event.go`
   - `types/state_diff.go`, `types/block_file.go`

### Reference
Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` — Step 1 for all type definitions.

### Modify
1. **Create `$ARGUMENTS/crates/rpc/rpc-eth-types/src/debank.rs`** with:

   **RLP-serialized types** (for state diff, matching Go's RLP encoding):
   - `BlockStorageDiff` — contains new_accounts, new_codes, account_storage_diffs
   - `NewAccount` — address_hash, account (slim RLP)
   - `NewCode` — code_hash, code bytes
   - `AccountStorageDiff` — address_hash, storage pairs
   - `IndexValuePair` — index hash, value bytes

   **JSON-serialized types** (for block file output):
   - `DebankBlock` — block metadata (number, hash, timestamp, etc.)
   - `DebankTransaction` — tx details with effective gas price
   - `DebankTrace` — call trace with ID, type, addresses, value, gas
   - `DebankEvent` — event log with topics, data, ID
   - `BlockFile` — contains block, transactions, traces, events
   - `BlockValidation` — hash-based integrity validation
   - `DebankOutPut` — final RPC output (block_file, header, state_diff, validation_hash)

   **Conversion implementations**:
   - `From<&CallTraceNode> for DebankTrace` — map revm-inspectors traces
   - `From<&CallLog> for DebankEvent` — map event logs
   - `build_debank_traces()` — recursive tree walker, assigns IDs, trace_address paths
   - `get_storage_diffs_from_cache()` — compute diff from CacheDB vs pre_db

   **ID generation** (must match Go):
   - `DebankID` trait using MD5 hash
   - `calc_validation_hash()` using SHA1

   **Genesis handling**:
   - `build_genesis_txs_and_traces()` — deterministic genesis output

2. **Export module** in `lib.rs`: `pub mod debank;`

3. **Add dependencies** to `Cargo.toml`:
   ```toml
   md-5.workspace = true
   sha1.workspace = true
   alloy-rlp.workspace = true
   alloy-genesis.workspace = true
   revm-bytecode.workspace = true
   ```

4. **Add workspace dependencies** to root `Cargo.toml` if needed.

### Verify
```bash
cd $ARGUMENTS && cargo check -p reth-rpc-eth-types
```

---

## Phase 2: Add StateDiffTraceDB

### Goal
Create a database wrapper that captures state changes for diff computation.

### Explore
1. Read `$ARGUMENTS/crates/rpc/rpc-eth-types/src/cache/db.rs`
2. Understand the existing `CacheDB`/`StateProviderDatabase` usage
3. Check `Database` and `DatabaseCommit` traits from `revm`

### Reference
Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` — Step 2.

### Modify
Add `StateDiffTraceDB` to `cache/db.rs`:

```rust
pub struct StateDiffTraceDB<ExtDB> {
    pub diff: InMemoryDB,   // Captures all state changes
    pub db: ExtDB,          // Real database for execution
}
```

Key trait implementations:
- `Database`: reads delegate to `self.db`
- `DatabaseCommit`: writes to **both** `self.diff` and `self.db`
- After execution, `self.diff.cache` has all changes for diff computation

### Verify
```bash
cd $ARGUMENTS && cargo check -p reth-rpc-eth-types
```

---

## Phase 3: Add trace_all_block Helper

### Goal
Add a method to replay all transactions in a block while collecting traces and state diff.

### Explore
1. Read `$ARGUMENTS/crates/rpc/rpc-eth-api/src/helpers/trace.rs`
2. Find the `Trace` trait
3. Understand existing `trace_block` or similar methods
4. Check how state providers are created from block IDs

### Reference
Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` — Step 3.

### Modify
Add `trace_all_block` method to the `Trace` trait:

```rust
fn trace_all_block<Setup, Insp, F, R>(
    &self,
    block_id: BlockId,
    inspector_setup: Setup,
    f: F,
) -> impl Future<Output = Result<(Vec<R>, BlockStorageDiff, Vec<Address>), Self::Error>>
```

Logic:
1. Create two state providers from parent block
2. Wrap one in `StateDiffTraceDB` for execution
3. Keep the other as `pre_db` for comparison
4. Replay all transactions with tracing inspector
5. After replay, extract diffs using `get_storage_diffs_from_cache()`

### Verify
```bash
cd $ARGUMENTS && cargo check -p reth-rpc-eth-api
```

---

## Phase 4: Define RPC Method

### Goal
Add `trace_debankBlock` to the TraceApi RPC trait.

### Explore
1. Read `$ARGUMENTS/crates/rpc/rpc-api/src/trace.rs`
2. Find the `TraceApi` trait with `#[rpc]` attribute
3. Understand the method naming convention (`#[method(name = "...")]`)

### Reference
Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` — Step 4.

### Modify
Add to `TraceApi` trait:
```rust
/// Returns debank's trace information for a given block.
#[method(name = "debankBlock")]
async fn trace_debank_block(&self, block_id: BlockId) -> RpcResult<DebankOutPut>;
```

### Verify
```bash
cd $ARGUMENTS && cargo check -p reth-rpc-api
```

---

## Phase 5: Implement trace_debank_block

### Goal
Implement the full `trace_debankBlock` RPC method.

### Explore
1. Read `$ARGUMENTS/crates/rpc/rpc/src/trace.rs`
2. Find the `TraceApi` impl
3. Understand how other trace methods (e.g., `trace_block`) are implemented
4. Check how blocks, receipts, and state are accessed

### Reference
Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` — Step 5.

### Modify
Implement `trace_debank_block`:

1. **Fetch block** and build `DebankBlock` + header
2. **Genesis block** (number == 0):
   - Build state diff from genesis alloc
   - Build genesis txs and traces via `build_genesis_txs_and_traces()`
   - Return early
3. **Fetch receipts** and build `DebankTransaction` list
4. **Empty block check**: If `parent_root == block_root`, return early with empty diff
5. **Replay block** with `TracingInspector`:
   ```rust
   let mut trace_cfg = TracingInspectorConfig::default_parity()
       .set_steps(true)
       .set_record_logs(true)
       .set_exclude_precompile_calls(false);
   trace_cfg.record_opcodes_filter = Some(OpcodeFilter::new().enabled(OpCode::SSTORE));
   ```
6. **Convert traces**: For each tx, call `build_debank_traces()` to convert `CallTraceArena` → Debank types
7. **Collect state diff** from `StateDiffTraceDB`
8. **Build and return** `DebankOutPut` with block_file, header, state_diff, validation_hash

Add `reth-rpc-eth-types` dependency to `crates/rpc/rpc/Cargo.toml`.

### Verify
```bash
cd $ARGUMENTS && cargo check -p reth-rpc
```

---

## Phase 6: Update Trait Bounds

### Goal
Ensure the RPC builder has correct trait bounds for `trace_debankBlock`.

### Explore
1. Read `$ARGUMENTS/crates/rpc/rpc-builder/src/lib.rs`
2. Find `register_trace` method
3. Check existing trait bounds

### Reference
Read `docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md` — Step 6.

### Modify
Update `register_trace` bounds to include `EthBlocks + LoadReceipt`:
```rust
pub fn register_trace(&mut self) -> &mut Self
where
    EthApi: TraceExt + EthBlocks + LoadReceipt,
```

### Verify
```bash
cd $ARGUMENTS && cargo check -p reth-rpc-builder
```

---

## Phase 7: Add Dependencies and Final Verification

### Goal
Ensure all workspace dependencies are configured and everything compiles.

### Explore
1. Read root `$ARGUMENTS/Cargo.toml` — check workspace dependencies section
2. Check for missing workspace dependency declarations

### Modify
1. Add to workspace `[dependencies]` in root `Cargo.toml` (if not already present):
   ```toml
   md-5 = "0.10"
   sha1 = "0.10"
   ```
2. Check and add any other missing workspace deps

### Final Verification
```bash
cd $ARGUMENTS && cargo build
```

If build fails, common issues:
- **Trait bound mismatches** — `StateDiffTraceDB` might need additional trait implementations
- **Type conversion errors** — Ensure all `From` impls handle edge cases
- **Missing workspace deps** — Add them to the root `Cargo.toml`
- **API changes in revm-inspectors** — Check if `CallTraceNode` fields have changed

### L2 Support (if Optimism)
Check if this is an Optimism reth build:
- Search for `optimism` in features or crate names
- If so, implement `get_deposit_nonce` in `crates/optimism/rpc/src/eth/receipt.rs`
- Add default `get_deposit_nonce` returning `None` in `crates/rpc/rpc-convert/src/transaction.rs`

---

## Completion

After all 7 phases, present summary:

```
## Adaptation Complete

### Files Created:
- crates/rpc/rpc-eth-types/src/debank.rs — Pipeline types in Rust

### Files Modified:
- crates/rpc/rpc-eth-types/src/lib.rs — Export debank module
- crates/rpc/rpc-eth-types/src/cache/db.rs — StateDiffTraceDB
- crates/rpc/rpc-eth-types/Cargo.toml — New dependencies
- crates/rpc/rpc-eth-api/src/helpers/trace.rs — trace_all_block helper
- crates/rpc/rpc-api/src/trace.rs — RPC method definition
- crates/rpc/rpc/src/trace.rs — Implementation
- crates/rpc/rpc/Cargo.toml — reth-rpc-eth-types dependency
- crates/rpc/rpc-builder/src/lib.rs — Updated trait bounds
- Cargo.toml — Workspace dependencies

### Key Differences from Go:
- RPC Tracer only (no Live Tracer, no S3/Kafka)
- StateDiffTraceDB (dual-DB) instead of OnCommit hook
- Types reimplemented in Rust (not embedded Go code)
- No core EVM or StateDB modifications

### Next Steps:
1. Build: `cargo build --release`
2. Start reth normally
3. Test RPC: `curl -X POST --data '{"method":"trace_debankBlock","params":["0x1"]}'`
```

Run the checklist from `references/checklist.md` to verify completeness.
