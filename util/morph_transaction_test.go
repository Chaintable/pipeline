package util

import (
	"math/big"
	"testing"

	"github.com/morph-l2/go-ethereum/common"
	"github.com/morph-l2/go-ethereum/core/types"
)

func TestMorphTransactionFeeFields(t *testing.T) {
	for _, tc := range []struct{ block, feeCap, tipCap, writerPrice int64 }{
		{19720201, 0x510bab7, 0x4f23637, 128225362},
		{29915105, 0xf4240, 0xf4240, 12035902},
		{30065944, 0x118c30, 0x118c30, 3979904},
	} {
		tx := types.NewTx(&types.MorphTx{GasFeeCap: big.NewInt(tc.feeCap), GasTipCap: big.NewInt(tc.tipCap)})
		receipt := &types.Receipt{EffectiveGasPrice: big.NewInt(tc.writerPrice)}
		got := BuildPipelineTransaction(tx, receipt, common.Address{}, nil)
		if got.GasFeeCap.Cmp(tx.GasFeeCap()) != 0 || got.GasTipCap.Cmp(tx.GasTipCap()) != 0 || got.GasPrice.Int64() != tc.writerPrice {
			t.Errorf("block %d: fee cap %v, tip cap %v, gas price %v", tc.block, got.GasFeeCap, got.GasTipCap, got.GasPrice)
		}
	}
}
