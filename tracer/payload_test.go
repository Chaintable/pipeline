package tracer

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

func payloadTraceFixture(t *testing.T, nonce uint64) (*PipelineTracer, *types.Block) {
	t.Helper()
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0), GasLimit: 100000, ParentHash: common.HexToHash("0x01")}
	collector := NewPayloadTracer(header)
	address := common.HexToAddress("0x1234")
	tx := types.NewTransaction(nonce, address, big.NewInt(1), 21000, big.NewInt(2), nil)
	receipt := &types.Receipt{Status: types.ReceiptStatusSuccessful, GasUsed: 21000, CumulativeGasUsed: 21000}
	collector.OnTxStart(tx, address)
	collector.OnLog(&types.Log{Address: address, Data: []byte{byte(nonce)}})
	collector.OnTxEnd(receipt, nil)
	header.Root = common.HexToHash("0x02")
	header.GasUsed = 21000
	block := types.NewBlockWithWithdrawals(header, []*types.Transaction{tx}, nil, []*types.Receipt{receipt}, nil, trie.NewStackTrie(nil))
	if err := collector.SealPayload(block); err != nil {
		t.Fatal(err)
	}
	return collector, block
}

func TestPayloadTraceIsolationAndTransfer(t *testing.T) {
	first, firstBlock := payloadTraceFixture(t, 1)
	second, secondBlock := payloadTraceFixture(t, 2)
	if first.block.BlockFile.Events[0].Data[0] != 1 || second.block.BlockFile.Events[0].Data[0] != 2 {
		t.Fatal("candidate traces shared their mutable context")
	}
	if first.block.BlockHeader.Hash != firstBlock.Hash() || first.block.BlockFile.Block.ID != firstBlock.Hash().Hex() {
		t.Fatal("trace metadata still contains the provisional block hash")
	}
	importer := &PipelineTracer{}
	if err := importer.AdoptPayload(first, secondBlock); err == nil {
		t.Fatal("adopted a different candidate")
	}
	if err := importer.AdoptPayload(first, firstBlock); err != nil {
		t.Fatal(err)
	}
	if first.MatchesPayload(firstBlock) || importer.block.BlockHash != firstBlock.Hash() {
		t.Fatal("trace ownership was not transferred")
	}
	if err := importer.AdoptPayload(first, firstBlock); err == nil {
		t.Fatal("consumed the same trace twice")
	}
	if !second.MatchesPayload(secondBlock) {
		t.Fatal("adoption modified another candidate")
	}
	// With no publishers initialized, this also checks that failure cannot upload.
	importer.OnBlockEnd(errors.New("state write failed"))
	if importer.block != nil {
		t.Fatal("failed import retained its artifact")
	}
}

func TestPayloadTraceCommitDoesNotPublish(t *testing.T) {
	collector, block := payloadTraceFixture(t, 1)
	collector.OnCommit(common.Hash{}, block.Root(), nil, nil, nil, nil, nil, nil)
	if collector.MatchesPayload(block) {
		t.Fatal("a collector that committed state remained reusable")
	}

	importer := &PipelineTracer{}
	importer.OnBlockStart(block)
	codeHash := common.HexToHash("0x03")
	code := []byte{1, 2, 3}
	importer.OnCommit(common.HexToHash("0x01"), block.Root(), nil, nil, nil, nil, nil, map[common.Hash][]byte{codeHash: code})
	if !importer.block.Committed || len(importer.block.BlockDiff.NewCodes) != 1 {
		t.Fatal("commit did not capture the artifact")
	}
	code[0] = 9
	if importer.block.BlockDiff.NewCodes[0].Code[0] != 1 {
		t.Fatal("artifact retained mutable commit input")
	}
	// OnCommit must work without S3/Kafka initialization; only a successful
	// OnBlockEnd is allowed to publish the captured artifact.
	importer.OnBlockEnd(errors.New("block write failed after state commit"))
}

func TestPayloadTraceSameRootAndIncomplete(t *testing.T) {
	collector, block := payloadTraceFixture(t, 1)
	importer := &PipelineTracer{}
	if err := importer.AdoptPayload(collector, block); err != nil {
		t.Fatal(err)
	}
	importer.OnCommit(block.Root(), block.Root(), nil, nil, nil, nil, nil, nil)
	if len(importer.block.BlockFile.Events) != 0 || len(importer.block.BlockFile.Txs) != 1 {
		t.Fatal("same-root output differs from the existing import semantics")
	}
	importer.OnBlockEnd(errors.New("test cleanup"))

	incomplete := NewPayloadTracer(block.Header())
	if err := incomplete.SealPayload(block); err == nil {
		t.Fatal("sealed a trace with missing transactions")
	}
	failed := NewPayloadTracer(block.Header())
	failed.OnTxStart(block.Transactions()[0], common.Address{})
	failed.OnTxEnd(nil, errors.New("invalid transaction"))
	if err := failed.SealPayload(block); err == nil {
		t.Fatal("sealed a failed execution")
	}
}

func TestPayloadCacheConfigDefaults(t *testing.T) {
	var config PayloadCacheConfig
	if err := config.normalize(); err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.MaxEntries != 2 || config.MaxBytes != 256*1024*1024 || config.TTLSeconds != 30 {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}
