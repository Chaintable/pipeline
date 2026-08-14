package util

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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

func TestBuildPipelineTransactionGasPrice(t *testing.T) {
	const feeDelegateDynamicFeeTxType = 22

	to := common.HexToAddress("0x20fdb3f41371ec0834a11dafcdb9acf5157236c4")
	feePayer := common.HexToAddress("0x189c141ed3df3b04a17e52cfe61eacf0563988e5")
	baseFee := big.NewInt(1)
	gasTipCap := big.NewInt(100000000000)
	gasFeeCap := big.NewInt(100000000002)
	dynamicFeeTx := types.DynamicFeeTx{
		ChainID:   big.NewInt(1111),
		Nonce:     8,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       807798,
		To:        &to,
		Value:     big.NewInt(0),
	}

	tests := []struct {
		name     string
		tx       *types.Transaction
		baseFee  *big.Int
		gasPrice *big.Int
	}{
		{
			name:     "dynamic fee uses effective gas price",
			tx:       types.NewTx(&dynamicFeeTx),
			baseFee:  baseFee,
			gasPrice: big.NewInt(100000000001),
		},
		{
			name: "dynamic fee is capped by max fee",
			tx: types.NewTx(&types.DynamicFeeTx{
				ChainID:   big.NewInt(1111),
				GasTipCap: big.NewInt(2),
				GasFeeCap: big.NewInt(100),
				Gas:       21000,
				To:        &to,
				Value:     big.NewInt(0),
			}),
			baseFee:  big.NewInt(99),
			gasPrice: big.NewInt(100),
		},
		{
			name: "fee delegated dynamic fee uses effective gas price",
			tx: types.NewTx(&types.FeeDelegateDynamicFeeTx{
				SenderTx: dynamicFeeTx,
				FeePayer: &feePayer,
			}),
			baseFee:  baseFee,
			gasPrice: big.NewInt(100000000001),
		},
		{
			name: "legacy fee is unchanged",
			tx: types.NewTx(&types.LegacyTx{
				GasPrice: big.NewInt(42),
				Gas:      21000,
				To:       &to,
				Value:    big.NewInt(0),
			}),
			baseFee:  big.NewInt(99),
			gasPrice: big.NewInt(42),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := BuildPipelineTransaction(test.tx, &types.Receipt{}, common.Address{}, test.baseFee)
			if got.GasPrice.Cmp(test.gasPrice) != 0 {
				t.Fatalf("gas price mismatch: got %v, want %v", got.GasPrice, test.gasPrice)
			}
			if test.tx.Type() == types.DynamicFeeTxType || test.tx.Type() == feeDelegateDynamicFeeTxType {
				if got.GasFeeCap.Cmp(test.tx.GasFeeCap()) != 0 {
					t.Fatalf("gas fee cap mismatch: got %v, want %v", got.GasFeeCap, test.tx.GasFeeCap())
				}
				if got.GasTipCap.Cmp(test.tx.GasTipCap()) != 0 {
					t.Fatalf("gas tip cap mismatch: got %v, want %v", got.GasTipCap, test.tx.GasTipCap())
				}
			}
		})
	}
}
