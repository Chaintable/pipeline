package tracer

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/core/vm"
)

func TestRPCTracerRejectsIncompleteTransaction(t *testing.T) {
	for _, tc := range []struct {
		name    string
		frames  []callFrame
		receipt *types.Receipt
		err     error
	}{
		{"missing root", nil, &types.Receipt{Status: types.ReceiptStatusFailed}, nil},
		{"unbalanced stack", []callFrame{{}, {}}, &types.Receipt{Status: types.ReceiptStatusSuccessful}, nil},
		{"missing receipt", []callFrame{{}}, nil, nil},
		{"missing root failure", []callFrame{{}}, &types.Receipt{Status: types.ReceiptStatusFailed}, nil},
		{"unexpected root failure", []callFrame{{Error: "execution reverted"}}, &types.Receipt{Status: types.ReceiptStatusSuccessful}, nil},
		{"execution validation error", []callFrame{{}}, &types.Receipt{Status: types.ReceiptStatusSuccessful}, errors.New("invalid nonce")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTestRPCTracer()
			tr.callTracer.callstack = tc.frames
			if err := tr.OnTxEnd(tc.receipt, tc.err); err == nil {
				t.Fatal("accepted incomplete transaction trace")
			}
			bf := tr.currentBlock.BlockFile
			if len(bf.Txs)+len(bf.Traces)+len(bf.ErrorTraces) != 0 {
				t.Fatal("invalid transaction was published")
			}
		})
	}
}

func TestRPCTracerAcceptsSuccessfulAndRevertedTransactions(t *testing.T) {
	for _, failed := range []bool{false, true} {
		tr := newTestRPCTracer()
		frame := callFrame{Type: vm.CALL, To: testAddr(1)}
		receipt := &types.Receipt{Status: types.ReceiptStatusSuccessful}
		if failed {
			frame.Error = "execution reverted"
			receipt.Status = types.ReceiptStatusFailed
		}
		tr.callTracer.callstack = []callFrame{frame}
		if err := tr.OnTxEnd(receipt, nil); err != nil {
			t.Fatal(err)
		}
		bf := tr.currentBlock.BlockFile
		if len(bf.Txs) != 1 || len(bf.Traces)+len(bf.ErrorTraces) != 1 || (len(bf.ErrorTraces) == 1) != failed {
			t.Fatalf("unexpected output for failed=%v: %+v", failed, bf)
		}
	}
}

func newTestRPCTracer() *RPCTracer {
	tr := NewRPCTracer()
	tr.OnBlockStart(types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1), Difficulty: new(big.Int)}))
	tr.OnTxStart(types.NewTransaction(0, *testAddr(1), new(big.Int), 100000, new(big.Int), nil), common.Address{})
	return tr
}
