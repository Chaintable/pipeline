# Adapting go-ethereum for Pipeline Integration

This guide is based on the adaptation from `v1.16.7` in the go-ethereum project. It describes how to integrate Pipeline into a standard go-ethereum (or compatible) execution client.

---

## Adaptation Overview

The following files require modification (based on go-ethereum v1.16.7):

| File | Change Type | Description |
|------|-------------|-------------|
| `core/tracing/hooks.go` | Add hook types | Add `CommitHook` and `BlockDBStartHook` |
| `core/state/statedb.go` | Add methods | Add `SetOnCommit`, `StateDiff`, commit callback |
| `core/state_processor.go` | Modify | Set receipt EffectiveGasPrice before OnTxEnd |
| `core/types/receipt.go` | Add method | Add `SetEffectiveGasPrice` |
| `core/blockchain.go` | Modify | Integrate pipeline hooks, Kafka push, reorg handling |
| `eth/tracers/live/pipeline.go` | New file | Register pipeline as a live tracer |
| `eth/api_debank.go` | New file | Implement `trace_debankBlock` RPC method |
| `eth/backend.go` | Modify | Register RPC API namespace |
| `go.mod` | Modify | Add pipeline dependency |

---

## Step 1: Extend Tracing Hooks (core/tracing/hooks.go)

Add two custom hook types for state commit callbacks and block database operation notifications.

### 1.1 Add Hook Type Definitions

Add the following after the `BlockHashReadHook` type definition:

```go
// CommitHook is called when the state is committed.
CommitHook = func(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, accountsOrigin map[common.Address][]byte, storages map[common.Hash]map[common.Hash][]byte, storagesOrigin map[common.Address]map[common.Hash][]byte, codes map[common.Hash][]byte)

BlockDBStartHook = func(StateDB)
```

### 1.2 Add Fields to the Hooks Struct

Append to the `Hooks` struct:

```go
// custom hook
OnCommit       CommitHook
OnBlockDBStart BlockDBStartHook
```

**Purpose:**
- `OnCommit`: Called when StateDB commits, providing complete state change data (destructed accounts, account changes, storage changes, new contract codes)
- `OnBlockDBStart`: Called before block processing writes to the database, allowing the pipeline to obtain a StateDB reference

---

## Step 2: Modify StateDB (core/state/statedb.go)

### 2.1 Add the onCommit Field

Add a field to the `StateDB` struct:

```go
onCommit tracing.CommitHook
```

Also add the `rlp` import.

### 2.2 Add the SetOnCommit Method

```go
func (s *StateDB) SetOnCommit(onCommit tracing.CommitHook) {
    s.onCommit = onCommit
}
```

### 2.3 Invoke onCommit in commitAndFlush

In the `commitAndFlush` method, after the snapshot commit completes and before the trie database commit, add the onCommit callback:

```go
if s.onCommit != nil {
    contracts := make(map[common.Hash][]byte)
    for _, code := range ret.codes {
        contracts[code.hash] = code.blob
    }
    destructs := make(map[common.Hash]struct{})
    accounts := make(map[common.Hash][]byte)
    for addr, v := range ret.accounts {
        if v == nil {
            destructs[addr] = struct{}{}
        } else {
            accounts[addr] = v
        }
    }
    s.onCommit(
        ret.originRoot,
        ret.root,
        destructs,
        accounts,
        ret.accountsOrigin,
        ret.storages,
        ret.storagesOrigin,
        contracts,
    )
}
```

### 2.4 Add the StateDiff Method (Required for RPC Tracer Mode)

This method is used in RPC Tracer mode to compute the state diff after block replay:

```go
func (s *StateDB) StateDiff(deleteEmptyObjects bool) (root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storages map[common.Hash]map[common.Hash][]byte, codes map[common.Hash][]byte, err error) {
    root = s.IntermediateRoot(deleteEmptyObjects)
    destructs = make(map[common.Hash]struct{})
    accounts = make(map[common.Hash][]byte)
    storages = make(map[common.Hash]map[common.Hash][]byte)
    codes = make(map[common.Hash][]byte)
    var (
        buf    = crypto.NewKeccakState()
        encode = func(val common.Hash) []byte {
            if val == (common.Hash{}) {
                return nil
            }
            blob, _ := rlp.EncodeToBytes(common.TrimLeftZeroes(val[:]))
            return blob
        }
    )
    for addr, prevObj := range s.stateObjectsDestruct {
        prev := prevObj.origin
        if prev == nil {
            continue
        }
        addrHash := crypto.HashData(buf, addr.Bytes())
        destructs[addrHash] = struct{}{}
    }
    for addr, op := range s.mutations {
        if op.isDelete() {
            continue
        }
        obj := s.stateObjects[addr]
        if obj == nil {
            panic("missing state object")
        }
        addrHash := crypto.HashData(buf, addr.Bytes())
        accounts[addrHash] = types.SlimAccountRLP(obj.data)
        if obj.dirtyCode {
            codes[common.Hash(obj.CodeHash())] = obj.code
        }
        for key, val := range obj.pendingStorage {
            if val == obj.originStorage[key] {
                continue
            }
            hash := crypto.HashData(buf, key[:])
            if _, ok := storages[addrHash]; !ok {
                storages[addrHash] = make(map[common.Hash][]byte)
            }
            storages[addrHash][hash] = encode(val)
        }
    }
    return
}
```

---

## Step 3: Fix Receipt EffectiveGasPrice (core/types/receipt.go + core/state_processor.go)

### 3.1 Add SetEffectiveGasPrice to Receipt

```go
func (r *Receipt) SetEffectiveGasPrice(tx *Transaction, baseFee *big.Int) {
    r.EffectiveGasPrice = tx.inner.effectiveGasPrice(new(big.Int), baseFee)
}
```

### 3.2 Call SetEffectiveGasPrice Before OnTxEnd in state_processor.go

Modify the `OnTxEnd` defer in `ApplyTransactionWithEVM`:

```go
// Before
if hooks.OnTxEnd != nil {
    defer func() { hooks.OnTxEnd(receipt, err) }()
}

// After
if hooks.OnTxEnd != nil {
    defer func() {
        receipt.SetEffectiveGasPrice(tx, evm.Context.BaseFee)
        hooks.OnTxEnd(receipt, err)
    }()
}
```

**Reason:** In upstream go-ethereum, `receipt.EffectiveGasPrice` is not populated when the `OnTxEnd` callback fires. Pipeline requires this field to correctly record the actual gas price paid by each transaction.

---

## Step 4: Modify BlockChain (core/blockchain.go)

This is the most critical and complex part of the adaptation.

### 4.1 Add Imports

```go
import (
    "github.com/Chaintable/pipeline/leader"
    "github.com/Chaintable/pipeline/tracer"
    "github.com/Chaintable/pipeline/util"
    ptypes "github.com/Chaintable/pipeline/types"
)
```

### 4.2 Inject Hooks in ProcessBlock

In the `ProcessBlock` method, before the `OnBlockEnd` registration, inject `OnBlockDBStart` and `OnCommit`:

```go
if bc.logger != nil && bc.logger.OnBlockDBStart != nil {
    bc.logger.OnBlockDBStart(statedb)
}
// ... existing OnBlockEnd code ...
if bc.logger != nil && bc.logger.OnCommit != nil {
    statedb.SetOnCommit(bc.logger.OnCommit)
}
```

**Purpose:**
- `OnBlockDBStart` provides the pipeline with a StateDB reference
- `SetOnCommit` injects the pipeline's `OnCommit` callback into StateDB, triggering data upload when state is committed

### 4.3 Add Kafka Push in writeBlockAndSetHead

After writing the head block and before sending the ChainEvent in `writeBlockAndSetHead`:

```go
isLeader := leader.GlobalManager.IsLeader()
leader.GlobalManager.RLock()
lastPushedBlock := tracer.NodeXPusher.LastPushedBlock()
leader.GlobalManager.RUnlock()

if tracer.NodeXPusher != nil && isLeader && lastPushedBlock.BlockNumber <= block.NumberU64() {
    _, dropBlocks, newBlocks := bc.getCommonAncestor(*lastPushedBlock, ptypes.BlockContext{
        BlockNumber: block.NumberU64(),
        Hash:        block.Hash(),
        ParentHash:  block.ParentHash(),
        Timestamp:   block.Time(),
    })
    // Build BlockChangeNotification and push to Kafka
    // changeType: 1=new block, 2=reorg (has dropped blocks)
    var blockChange *ptypes.BlockChangeNotification
    if len(dropBlocks) > 0 {
        blockChange = &ptypes.BlockChangeNotification{ChangeType: 2, NewBlocks: newBlocks, DropBlocks: dropBlocks}
    } else if len(newBlocks) > 0 {
        blockChange = &ptypes.BlockChangeNotification{ChangeType: 1, NewBlocks: newBlocks}
    }

    // Handle empty blocks (when parent.Root == block.Root, no state changes occurred, manually trigger OnCommit)
    parent := bc.GetHeaderByHash(block.Header().ParentHash)
    if parent.Root == block.Root() {
        bc.logger.OnCommit(parent.Root, block.Root(), nil, nil, nil, nil, nil, nil)
    }

    if blockChange != nil {
        tracer.NodeXPusher.PushBlockChangeNotification(blockChange)
    }
}
```

### 4.4 Add the Same Logic in SetCanonical

The `SetCanonical` method also requires the same Kafka push logic to handle chain reorganization scenarios.

### 4.5 Add the getCommonAncestor Helper Method

This method computes the common ancestor of two blocks and returns the paths from the ancestor to each block (i.e., the dropped and new block lists):

```go
func (bc *BlockChain) getCommonAncestor(blocka ptypes.BlockContext, blockb ptypes.BlockContext) (ptypes.BlockContext, []ptypes.BlockContext, []ptypes.BlockContext) {
    // 1. Fast path: blockb is a direct child of blocka
    // 2. Walk blockb back to the same height as blocka
    // 3. Walk both blocks back simultaneously until hashes match
    // Returns: (ancestor, dropBlocks, newBlocks)
}
```

### 4.6 Add the GetHeaderByHash2 Helper Method

Falls back to downloading block headers from S3 when they are not available locally:

```go
func (bc *BlockChain) GetHeaderByHash2(blockHash common.Hash) *types.Header {
    header := bc.GetHeaderByHash(blockHash)
    if header == nil {
        if tracer.NodeXPusher != nil {
            header := &types.Header{}
            err := util.DownloadFileFromS3Json(tracer.NodeXPusher.Uploader, tracer.NodeXPusher.Bucket,
                fmt.Sprintf("%s/%s/block", tracer.BizChainID, blockHash.String()), header)
            if err != nil {
                return nil
            }
            return header
        }
    }
    return header
}
```

---

## Step 5: Register the Live Tracer (eth/tracers/live/pipeline.go)

Create a new file to register the pipeline tracer as a go-ethereum live tracer:

```go
package live

import (
    "encoding/json"
    "github.com/Chaintable/pipeline/tracer"
    "github.com/ethereum/go-ethereum/core/tracing"
    "github.com/ethereum/go-ethereum/eth/tracers"
)

func init() {
    tracers.LiveDirectory.Register("pipeline", NewPipelineTracer)
}

func NewPipelineTracer(cfg json.RawMessage) (*tracing.Hooks, error) {
    t, err := tracer.NewPipelineTracer(cfg)
    if err != nil {
        return nil, err
    }
    return &tracing.Hooks{
        OnBlockchainInit: t.OnBlockchainInit,
        OnClose:          t.OnClose,
        OnBlockStart:     t.OnBlockStart,
        OnTxStart:        t.OnTxStart,
        OnTxEnd:          t.OnTxEnd,
        OnEnter:          t.OnEnter,
        OnExit:           t.OnExit,
        OnLog:            t.OnLog,
        OnOpcode:         t.OnOpcode,
        OnGenesisBlock:   t.OnGenesisBlock,
        OnCommit:         t.OnCommit,
        OnBalanceChange:  t.OnBalanceChange,
        OnBlockDBStart:   t.OnBlockDBStart,
    }, nil
}
```

Enable at geth startup with `--vmtrace pipeline --vmtrace.jsonconfig '{...}'`.

---

## Step 6: Implement the RPC Interface (eth/api_debank.go + eth/backend.go)

### 6.1 Create eth/api_debank.go

Implement the `trace_debankBlock` RPC method:

```go
package eth

type DebankAPI struct {
    eth *Ethereum
}

func NewDebankAPI(eth *Ethereum) *DebankAPI {
    return &DebankAPI{eth: eth}
}

func (api *DebankAPI) DebankBlock(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*ptypes.DebankOutPut, error) {
    // 1. Fetch the target block
    // 2. If genesis block, handle specially (construct genesis tx/trace/state diff)
    // 3. Get the parent block state
    // 4. Create RPCTracer and register hooks
    // 5. Replay the block using Processor.Process
    // 6. Call statedb.StateDiff() to obtain the state diff
    // 7. Verify state root matches
    // 8. Call rpcTracer.GetOutPut() and return the result
}
```

### 6.2 Register the API in eth/backend.go

Add to the `APIs()` method:

```go
{
    Namespace: "trace",
    Service:   NewDebankAPI(s),
},
```

---

## Step 7: Add Dependencies (go.mod)

```bash
go get github.com/Chaintable/pipeline@latest
```

---

## Adaptation Checklist

### Required (Live Tracer Mode)

- [ ] `core/tracing/hooks.go` - Add `CommitHook` and `BlockDBStartHook` types
- [ ] `core/tracing/hooks.go` - Add `OnCommit` and `OnBlockDBStart` to the `Hooks` struct
- [ ] `core/state/statedb.go` - Add `onCommit` field and `SetOnCommit` method
- [ ] `core/state/statedb.go` - Invoke `onCommit` in `commitAndFlush`
- [ ] `core/types/receipt.go` - Add `SetEffectiveGasPrice` method
- [ ] `core/state_processor.go` - Call `SetEffectiveGasPrice` before `OnTxEnd`
- [ ] `core/blockchain.go` - Inject `OnBlockDBStart` and `OnCommit` in `ProcessBlock`
- [ ] `core/blockchain.go` - Add Kafka push in `writeBlockAndSetHead`
- [ ] `core/blockchain.go` - Add Kafka push in `SetCanonical`
- [ ] `core/blockchain.go` - Add `getCommonAncestor` and `GetHeaderByHash2` methods
- [ ] `eth/tracers/live/pipeline.go` - Register the live tracer
- [ ] `go.mod` - Add pipeline dependency

### Optional (RPC Tracer Mode)

- [ ] `core/state/statedb.go` - Add `StateDiff` method
- [ ] `eth/api_debank.go` - Implement `trace_debankBlock` endpoint
- [ ] `eth/backend.go` - Register `trace` namespace API

### Build & CI

- [ ] `Dockerfile.debank` - Build image (optional)
- [ ] `.github/workflows/build.debank.yml` - CI build pipeline (optional)
- [ ] `.github/workflows/release.debank.yml` - Release pipeline (optional)

---

## Notes for Non-Standard Chains

When adapting non-standard Ethereum chains (e.g., Scroll, XDC, Arbitrum Nitro, L2geth), keep the following in mind:

1. **chainVariant configuration**: Set the `chainVariant` field in the tracer config JSON
2. **StateDB structural differences**: Different chains may have different StateDB internals; the location and parameters of `commitAndFlush` may need adjustment
3. **Receipt fields**: Some chains may have additional receipt fields
4. **BlockChain methods**: Method signatures for `ProcessBlock`, `writeBlockAndSetHead`, and `SetCanonical` may differ
5. **Genesis handling**: Different chains may have different genesis formats; the genesis handling logic in the RPC Tracer needs to be adapted accordingly

---

## Configuration Example

Pipeline tracer configuration JSON for geth startup:

```json
{
    "region": "us-east-1",
    "node_x_bucket": "my-nodex-bucket",
    "chain_table_bucket": "my-chaintable-bucket",
    "brokers": ["kafka1:9092", "kafka2:9092"],
    "topic": "block-changes",
    "s3_temp_dir": "/tmp/pipeline",
    "version": "v1",
    "enable_prestate_tracer": false,
    "etcd_endpoints": ["etcd1:2379", "etcd2:2379"],
    "election_key": "/pipeline/leader/eth",
    "grace_period": 10
}
```

Launch via geth command line:

```bash
geth --vmtrace pipeline --vmtrace.jsonconfig '{"region":"us-east-1",...}'
```
