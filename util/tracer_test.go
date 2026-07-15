package util

import (
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type testDepositReceipt struct {
	nonce *uint64
}

func (r *testDepositReceipt) DepositNonceValue() *uint64 {
	return r.nonce
}

func TestTransactionNonce(t *testing.T) {
	depositNonce := uint64(55)
	tests := []struct {
		name    string
		raw     uint64
		receipt any
		want    uint64
	}{
		{name: "deposit nonce", receipt: &testDepositReceipt{nonce: &depositNonce}, want: depositNonce},
		{name: "raw nonce wins", raw: 7, receipt: &testDepositReceipt{nonce: &depositNonce}, want: 7},
		{name: "nil deposit nonce", receipt: &testDepositReceipt{}, want: 0},
		{name: "standard receipt", receipt: &types.Receipt{}, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := transactionNonce(test.raw, test.receipt); got != test.want {
				t.Fatalf("unexpected nonce: want %d, got %d", test.want, got)
			}
		})
	}
}

func TestBuildPipelineTransactionPreservesUint64Nonce(t *testing.T) {
	to := common.HexToAddress("0x1")
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    math.MaxUint64,
		To:       &to,
		Gas:      21_000,
		GasPrice: big.NewInt(1),
		Value:    big.NewInt(0),
	})
	receipt := &types.Receipt{Status: types.ReceiptStatusSuccessful}

	got := BuildPipelineTransaction(tx, receipt, common.Address{}, big.NewInt(0))
	want := new(big.Int).SetUint64(math.MaxUint64)
	if got.Nonce.Cmp(want) != 0 {
		t.Fatalf("unexpected nonce: want %s, got %s", want, got.Nonce)
	}
}
