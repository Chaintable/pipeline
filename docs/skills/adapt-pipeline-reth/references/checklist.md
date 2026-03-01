# Reth Adaptation Checklist

Use this checklist to verify the Pipeline adaptation is complete. Replace `$TARGET` with the target repository path.

## Core Types (debank.rs)

- [ ] `crates/rpc/rpc-eth-types/src/debank.rs` exists
- [ ] `crates/rpc/rpc-eth-types/src/lib.rs` exports `debank` module
- [ ] RLP types: `BlockStorageDiff`, `NewAccount`, `NewCode`, `AccountStorageDiff`, `IndexValuePair`
- [ ] JSON types: `DebankBlock`, `DebankTransaction`, `DebankTrace`, `DebankEvent`
- [ ] Output types: `BlockFile`, `BlockValidation`, `DebankOutPut`
- [ ] `From<&CallTraceNode> for DebankTrace` implementation
- [ ] `From<&CallLog> for DebankEvent` implementation
- [ ] `build_debank_traces()` function
- [ ] `get_storage_diffs_from_cache()` function
- [ ] `DebankID` trait with MD5 hashing
- [ ] `calc_validation_hash()` with SHA1
- [ ] `build_genesis_txs_and_traces()` for genesis blocks

## StateDiffTraceDB

- [ ] `crates/rpc/rpc-eth-types/src/cache/db.rs` — `StateDiffTraceDB` struct
- [ ] `Database` trait implemented (reads from `db`)
- [ ] `DatabaseCommit` trait implemented (writes to both `diff` and `db`)

## RPC Method

- [ ] `crates/rpc/rpc-eth-api/src/helpers/trace.rs` — `trace_all_block` method
- [ ] `crates/rpc/rpc-api/src/trace.rs` — `trace_debankBlock` defined in `TraceApi`
- [ ] `crates/rpc/rpc/src/trace.rs` — `trace_debank_block` implemented
- [ ] Genesis block handling (number == 0)
- [ ] Empty block handling (parent_root == block_root)
- [ ] TracingInspector configured with SSTORE filter

## Trait Bounds

- [ ] `crates/rpc/rpc-builder/src/lib.rs` — `register_trace` has `EthBlocks + LoadReceipt` bounds

## Dependencies

- [ ] `crates/rpc/rpc-eth-types/Cargo.toml` — md-5, sha1, alloy-rlp, alloy-genesis, revm-bytecode
- [ ] `crates/rpc/rpc/Cargo.toml` — reth-rpc-eth-types
- [ ] Root `Cargo.toml` — workspace dependencies for md-5, sha1

## L2-Specific (if Optimism)

- [ ] `crates/rpc/rpc-convert/src/transaction.rs` — `get_deposit_nonce` trait method
- [ ] `crates/rpc/rpc-eth-types/src/receipt.rs` — default `get_deposit_nonce` (returns None)
- [ ] `crates/optimism/rpc/src/eth/receipt.rs` — OP-specific `get_deposit_nonce`

## NOT Required (compared to go-ethereum)

- No `core/tracing/` equivalent
- No `core/state/` modifications
- No `core/blockchain.go` changes
- No embedded pipeline source code
- No S3/Kafka integration
- No leader election
- No CLI flags

## Build Verification

- [ ] `cargo check` succeeds
- [ ] `cargo build` succeeds
- [ ] `cargo test -p reth-rpc-eth-types` passes (if tests exist)

## Verification Commands

```bash
# Check debank.rs exists
test -f $TARGET/crates/rpc/rpc-eth-types/src/debank.rs && echo "OK" || echo "MISSING"

# Check module export
grep -n "pub mod debank" $TARGET/crates/rpc/rpc-eth-types/src/lib.rs

# Check StateDiffTraceDB
grep -n "StateDiffTraceDB" $TARGET/crates/rpc/rpc-eth-types/src/cache/db.rs

# Check RPC definition
grep -n "debankBlock" $TARGET/crates/rpc/rpc-api/src/trace.rs

# Check trace_all_block
grep -n "trace_all_block" $TARGET/crates/rpc/rpc-eth-api/src/helpers/trace.rs

# Check implementation
grep -n "trace_debank_block" $TARGET/crates/rpc/rpc/src/trace.rs

# Check trait bounds
grep -n "EthBlocks" $TARGET/crates/rpc/rpc-builder/src/lib.rs

# Check dependencies
grep "md-5" $TARGET/crates/rpc/rpc-eth-types/Cargo.toml
grep "sha1" $TARGET/crates/rpc/rpc-eth-types/Cargo.toml

# Check ID compatibility
grep -n "Md5::new" $TARGET/crates/rpc/rpc-eth-types/src/debank.rs
grep -n "Sha1::new" $TARGET/crates/rpc/rpc-eth-types/src/debank.rs
```
