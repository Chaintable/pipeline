package tracer

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state/snapshot"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// replayTx feeds one transaction's execution events to tr, the way the EVM
// reports them during import (PipelineTracer) or replay (RPCTracer).
func replayTx(evm vm.EVMLogger, onLog func(*types.Log), from, to, callee common.Address) {
	evm.CaptureTxStart(100000)
	evm.CaptureStart(nil, from, to, false, []byte{0x01, 0x02}, 100000, big.NewInt(7))
	evm.CaptureState(0, vm.SSTORE, 0, 0, nil, nil, 1, nil)
	evm.CaptureEnter(vm.CALL, to, callee, []byte{0x03}, 50000, big.NewInt(0))
	onLog(&types.Log{Address: callee, Topics: []common.Hash{{0xaa}, {0xbb}}, Data: []byte{0x04}, Index: 0})
	evm.CaptureExit([]byte{0x05}, 21000, nil)
	evm.CaptureEnter(vm.CALL, to, callee, nil, 1000, big.NewInt(0))
	evm.CaptureExit(nil, 1000, vm.ErrExecutionReverted)
	evm.CaptureEnd([]byte{0x06}, 60000, 0, nil)
	evm.CaptureTxEnd(40000)
}

func testBlock(t *testing.T) (*types.Block, *types.Transaction, *types.Receipt) {
	t.Helper()
	to := common.HexToAddress("0x1000")
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(1111),
		Nonce:     3,
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(100),
		Gas:       100000,
		To:        &to,
		Value:     big.NewInt(7),
		Data:      []byte{0x01, 0x02},
	})
	header := &types.Header{
		ParentHash: common.Hash{0x01},
		Number:     big.NewInt(42),
		Difficulty: big.NewInt(1),
		GasLimit:   30000000,
		GasUsed:    60000,
		Time:       1700000000,
		BaseFee:    big.NewInt(10),
		Root:       common.Hash{0x02},
	}
	block := types.NewBlockWithHeader(header).WithBody([]*types.Transaction{tx}, nil)
	receipt := &types.Receipt{Type: tx.Type(), Status: types.ReceiptStatusSuccessful, GasUsed: 60000, TxHash: tx.Hash(), TransactionIndex: 0}
	return block, tx, receipt
}

// testStateChanges returns one account and one storage slot, so that the
// encoded diff does not depend on map iteration order.
func testStateChanges(t *testing.T) (map[common.Hash]struct{}, map[common.Hash][]byte, map[common.Hash]map[common.Hash][]byte, map[common.Hash][]byte) {
	t.Helper()
	code := []byte{0x60, 0x00}
	codeHash := crypto.Keccak256Hash(code)
	account := crypto.Keccak256Hash(common.HexToAddress("0x1000").Bytes())
	slot, err := rlp.EncodeToBytes(common.TrimLeftZeroes(common.Hash{31: 9}.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	accounts := map[common.Hash][]byte{
		account: snapshot.SlimAccountRLP(5, big.NewInt(1000), common.Hash{}, codeHash.Bytes()),
	}
	storages := map[common.Hash]map[common.Hash][]byte{account: {{0x01}: slot}}
	codes := map[common.Hash][]byte{codeHash: code}
	return map[common.Hash]struct{}{}, accounts, storages, codes
}

func TestPipelineTracerMatchesRPCTracer(t *testing.T) {
	block, tx, receipt := testBlock(t)
	from := common.HexToAddress("0x2000")
	to := *tx.To()
	callee := common.HexToAddress("0x3000")
	parentRoot := common.Hash{0x0f}
	destructs, accounts, storages, codes := testStateChanges(t)

	rpc := NewRPCTracer()
	rpc.OnBlockStart(block)
	rpc.OnTxStart(tx, from)
	replayTx(rpc.GetCallTracer(), rpc.OnLog, from, to, callee)
	rpc.OnTxEnd(receipt, nil)
	want := rpc.GetOutPut(parentRoot, block.Root(), destructs, accounts, storages, codes)

	live := &PipelineTracer{}
	live.OnBlockStart(block)
	live.OnTxStart(tx, from)
	replayTx(live, live.OnLog, from, to, callee)
	live.OnTxEnd(receipt, nil)
	live.OnCommit(parentRoot, block.Root(), destructs, accounts, storages, codes)
	got := live.block

	if !got.Committed {
		t.Fatal("block not marked committed")
	}
	if !reflect.DeepEqual(got.BlockHeader, want.Header) {
		t.Errorf("header mismatch\n got %+v\nwant %+v", got.BlockHeader, want.Header)
	}
	gotDiff, err := util.EncodeToRlp(got.BlockDiff)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDiff, want.StateDiff) {
		t.Errorf("state diff mismatch\n got %x\nwant %x", gotDiff, []byte(want.StateDiff))
	}
	if got.BlockFile.Validation().ValidationHash != want.ValidationHash {
		t.Errorf("validation hash = %d, want %d", got.BlockFile.Validation().ValidationHash, want.ValidationHash)
	}
	// The block files differ only in when processing started.
	gotFile, wantFile := *got.BlockFile, *want.BlockFile
	gotFile.Block.ProcessStartTimestamp, wantFile.Block.ProcessStartTimestamp = 0, 0
	if !reflect.DeepEqual(gotFile, wantFile) {
		g, _ := json.Marshal(gotFile)
		w, _ := json.Marshal(wantFile)
		t.Errorf("block file mismatch\n got %s\nwant %s", g, w)
	}
	if len(gotFile.Txs) != 1 || gotFile.Txs[0].GasPrice.Cmp(big.NewInt(12)) != 0 {
		t.Errorf("txs = %+v, want one tx with effective gas price 12", gotFile.Txs)
	}
	if len(gotFile.Traces) != 2 || len(gotFile.ErrorTraces) != 1 || len(gotFile.Events) != 1 {
		t.Errorf("traces=%d errorTraces=%d events=%d, want 2/1/1", len(gotFile.Traces), len(gotFile.ErrorTraces), len(gotFile.Events))
	}
	if want := []string{strings.ToLower(to.Hex())}; !reflect.DeepEqual(gotFile.StorageContracts, want) {
		t.Errorf("storage contracts = %v, want %v", gotFile.StorageContracts, want)
	}
}

func TestPipelineTracerUnchangedRootHasNoDiff(t *testing.T) {
	block, _, _ := testBlock(t)
	live := &PipelineTracer{}
	live.OnBlockStart(block)
	live.OnCommit(block.Root(), block.Root(), nil, nil, nil, nil)
	if !live.block.Committed {
		t.Fatal("block not marked committed")
	}
	if live.block.BlockDiff != nil {
		t.Errorf("state diff = %+v, want none for an unchanged root", live.block.BlockDiff)
	}
}

func TestPipelineTracerIgnoresEventsOutsideBlock(t *testing.T) {
	_, tx, receipt := testBlock(t)
	live := &PipelineTracer{}
	live.OnTxStart(tx, common.Address{})
	replayTx(live, live.OnLog, common.Address{}, *tx.To(), common.Address{})
	live.OnTxEnd(receipt, nil)
	live.OnCommit(common.Hash{1}, common.Hash{2}, nil, nil, nil, nil)
	live.OnBlockEnd(nil)
	if live.block != nil || live.callTracer != nil || live.published {
		t.Errorf("tracer recorded events without a block: %+v", live)
	}
}

type testChain struct {
	headers map[common.Hash]*types.Header
}

func (c *testChain) extend(parent *types.Header, fork byte) *types.Header {
	h := &types.Header{
		ParentHash: parent.Hash(),
		Number:     new(big.Int).Add(parent.Number, big.NewInt(1)),
		Difficulty: big.NewInt(1),
		Time:       parent.Time + 1,
		Extra:      []byte{fork},
	}
	c.headers[h.Hash()] = h
	return h
}

func (c *testChain) get(hash common.Hash) *types.Header {
	return c.headers[hash]
}

func TestBlockChangeForHead(t *testing.T) {
	c := &testChain{headers: make(map[common.Hash]*types.Header)}
	root := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(1)}
	c.headers[root.Hash()] = root
	a1 := c.extend(root, 'a')
	a2 := c.extend(a1, 'a')
	a3 := c.extend(a2, 'a')
	b2 := c.extend(a1, 'b')
	b3 := c.extend(b2, 'b')
	b4 := c.extend(b3, 'b')

	ctxs := func(headers ...*types.Header) []ptypes.BlockContext {
		var out []ptypes.BlockContext
		for _, h := range headers {
			out = append(out, blockContext(h))
		}
		return out
	}
	tests := []struct {
		name string
		last *types.Header
		head *types.Header
		want *ptypes.BlockChangeNotification
	}{
		{"extend", a1, a3, &ptypes.BlockChangeNotification{ChangeType: 1, NewBlocks: ctxs(a2, a3)}},
		{"unchanged", a3, a3, nil},
		{"rewind not announced", a3, a2, nil},
		{"longer fork", a3, b4, &ptypes.BlockChangeNotification{ChangeType: 2, NewBlocks: ctxs(b2, b3, b4), DropBlocks: ctxs(a2, a3)}},
		{"same height fork", a3, b3, &ptypes.BlockChangeNotification{ChangeType: 2, NewBlocks: ctxs(b2, b3), DropBlocks: ctxs(a2, a3)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := blockChangeForHead(blockContext(tt.last), tt.head, c.get)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}

	t.Run("missing parent", func(t *testing.T) {
		delete(c.headers, a2.Hash())
		if _, err := blockChangeForHead(blockContext(a1), a3, c.get); err == nil {
			t.Error("expected an error for a missing parent header")
		}
	})
}

func TestGenesisOutput(t *testing.T) {
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(0), Difficulty: big.NewInt(1), Root: common.Hash{0x0a}})
	funded := common.HexToAddress("0x02")
	contract := common.HexToAddress("0x01")
	alloc := core.GenesisAlloc{
		funded:   {Balance: big.NewInt(10)},
		contract: {Balance: big.NewInt(0), Code: []byte{0x60}, Storage: map[common.Hash]common.Hash{{0x01}: {0x02}}},
	}
	out := GenesisOutput(block, alloc)

	var ids []string
	for _, tx := range out.BlockFile.Txs {
		ids = append(ids, tx.ID[:11]+tx.ID[24:])
	}
	wantIDs := []string{
		"0xgenesis02" + strings.ToLower(contract.Hex()),
		"0xgenesis01" + strings.ToLower(funded.Hex()),
		"0xgenesis03" + genesisNativeToken,
	}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("tx ids = %v, want %v", ids, wantIDs)
	}
	if len(out.BlockFile.Traces) != 3 {
		t.Errorf("traces = %d, want 3", len(out.BlockFile.Traces))
	}
	if want := []string{strings.ToLower(contract.Hex())}; !reflect.DeepEqual(out.BlockFile.StorageContracts, want) {
		t.Errorf("storage contracts = %v, want %v", out.BlockFile.StorageContracts, want)
	}
	if len(out.StateDiff) == 0 {
		t.Error("state diff is empty")
	}
	_, diff, _ := genesisArtifacts(block, alloc)
	if diff.Hash != block.Root() || len(diff.NewAccounts) != 2 || len(diff.NewCodes) != 1 {
		t.Errorf("state diff = %+v, want root %s with 2 accounts and 1 code", diff, block.Root())
	}
	if out.ValidationHash != out.BlockFile.Validation().ValidationHash {
		t.Errorf("validation hash = %d, want %d", out.ValidationHash, out.BlockFile.Validation().ValidationHash)
	}
}

func TestNewPipelineTracerConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{"manual leader", `{"region":"r","node_x_bucket":"n","chain_table_bucket":"c","brokers":["b"],"is_backup":false,"node_id":"w"}`, false},
		{"failover", `{"region":"r","node_x_bucket":"n","chain_table_bucket":"c","brokers":["b"],"etcd_endpoints":["e"],"node_id":"w"}`, false},
		{"missing bucket", `{"region":"r","node_x_bucket":"n","brokers":["b"],"is_backup":false}`, true},
		{"no role", `{"region":"r","node_x_bucket":"n","chain_table_bucket":"c","brokers":["b"]}`, true},
		{"both roles", `{"region":"r","node_x_bucket":"n","chain_table_bucket":"c","brokers":["b"],"is_backup":true,"etcd_endpoints":["e"]}`, true},
		{"bad json", `{`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewPipelineTracer(json.RawMessage(tt.config))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (tr.config.GracePeriod != 10 || tr.config.WriterRegistryTTL != 10) {
				t.Errorf("defaults not applied: %+v", tr.config)
			}
		})
	}
}
