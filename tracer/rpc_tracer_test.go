package tracer

import (
	"errors"
	"math/big"
	"testing"

	"github.com/XinFinOrg/XDPoSChain/common"
	"github.com/XinFinOrg/XDPoSChain/core/types"
	"github.com/XinFinOrg/XDPoSChain/core/vm"
)

func TestRPCTracerSkipEVMTransaction(t *testing.T) {
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1), Time: big.NewInt(1), Difficulty: big.NewInt(1)})
	from, to := common.Address{1}, common.Address{2}
	tx := types.NewTransaction(0, to, big.NewInt(7), 100000, big.NewInt(1), []byte{0x12})
	tr := &RPCTracer{}
	tr.OnBlockStart(block)
	tr.OnSkipEvmTxStart(tx, from)
	tr.OnTxEnd(&types.Receipt{Status: types.ReceiptStatusSuccessful, TxHash: tx.Hash()}, nil)
	out := tr.GetOutPut(types.EmptyRootHash, types.EmptyRootHash, nil, nil, nil, nil)
	bf := out.BlockFile
	if len(bf.Txs) != 1 || len(bf.Traces) != 1 || len(bf.ErrorTraces) != 0 {
		t.Fatalf("skipped EVM tx lost its synthetic root: %+v", bf.Validation())
	}
	if bf.Traces[0].TxID != tx.Hash().Hex() || bf.Traces[0].CallCreateType != "call" || bf.Traces[0].GasUsed.Sign() != 0 || (*big.Int)(bf.Traces[0].Value).Cmp(tx.Value()) != 0 {
		t.Fatalf("incorrect skipped EVM trace: %+v", bf.Traces[0])
	}
	if len(out.StateDiff) != 0 || out.ValidationHash != bf.Validation().ValidationHash {
		t.Fatal("unchanged root must have no state diff and valid record checksum")
	}
}

func TestRPCTracerLegacyCallbacksPropagateParentFailure(t *testing.T) {
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1), Time: big.NewInt(1), Difficulty: big.NewInt(1)})
	from, to, child := common.Address{1}, common.Address{2}, common.Address{3}
	tx := types.NewTransaction(0, to, big.NewInt(0), 100000, big.NewInt(1), nil)
	tr := &RPCTracer{}
	tr.OnBlockStart(block)
	tr.OnTxStart(tx, from)
	tr.CaptureStart(nil, from, to, false, nil, 79000, big.NewInt(0))
	tr.CaptureEnter(vm.CALL, to, child, nil, 10000, big.NewInt(0))
	tr.CaptureExit(nil, 100, nil)
	tr.CaptureEnd(nil, 1000, 0, errors.New("execution reverted"))
	tr.OnTxEnd(&types.Receipt{Status: types.ReceiptStatusFailed, GasUsed: 22000, TxHash: tx.Hash()}, nil)
	out := tr.GetOutPut(types.EmptyRootHash, types.EmptyRootHash, nil, nil, nil, nil)
	bf := out.BlockFile
	if len(bf.Txs) != 1 || bf.Txs[0].Status || len(bf.Traces) != 0 || len(bf.ErrorTraces) != 2 {
		t.Fatalf("legacy callbacks lost failure inheritance: %+v", bf.Validation())
	}
	if bf.ErrorTraces[0].Error != "execution reverted" || bf.ErrorTraces[1].Error != "parent call failed" {
		t.Fatalf("incorrect trace errors: %+v", bf.ErrorTraces)
	}
}
