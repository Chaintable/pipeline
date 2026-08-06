package util

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestBuildPilelineBlockHeaderPreservesWemixFields(t *testing.T) {
	wemixHeader := &types.Header{
		Number:       big.NewInt(123),
		Difficulty:   big.NewInt(1),
		Fees:         big.NewInt(456),
		Rewards:      []byte{0x01, 0x02},
		MinerNodeId:  []byte{0x03, 0x04},
		MinerNodeSig: []byte{0x05, 0x06},
	}

	got := BuildPilelineBlockHeader(types.NewBlockWithHeader(wemixHeader))

	if got.Fees == nil || got.Fees.ToInt().Cmp(wemixHeader.Fees) != 0 {
		t.Fatalf("fees mismatch: got %v, want %v", got.Fees, wemixHeader.Fees)
	}
	if !bytes.Equal(got.Rewards, wemixHeader.Rewards) {
		t.Fatalf("rewards mismatch: got %x, want %x", got.Rewards, wemixHeader.Rewards)
	}
	if !bytes.Equal(got.MinerNodeID, wemixHeader.MinerNodeId) {
		t.Fatalf("miner node ID mismatch: got %x, want %x", got.MinerNodeID, wemixHeader.MinerNodeId)
	}
	if !bytes.Equal(got.MinerNodeSig, wemixHeader.MinerNodeSig) {
		t.Fatalf("miner node signature mismatch: got %x, want %x", got.MinerNodeSig, wemixHeader.MinerNodeSig)
	}
}
