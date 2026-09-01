package tracer

import (
	"bytes"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestBuildGenesisSyntheticTransactions(t *testing.T) {
	addr1 := common.HexToAddress("0x0000000000000000000000000000000000000001")
	addr2 := common.HexToAddress("0x0000000000000000000000000000000000000002")
	addr3 := common.HexToAddress("0x0000000000000000000000000000000000000003")
	code := []byte{0x60, 0x00}
	state := types.GenesisAlloc{
		addr3: {Balance: nil},
		addr2: {Balance: big.NewInt(9)},
		addr1: {Balance: big.NewInt(7), Code: code},
	}
	balanceBefore := new(big.Int).Set(state[addr1].Balance)
	codeBefore := append([]byte(nil), state[addr1].Code...)

	txs, traces := BuildGenesisSyntheticTransactions(state)
	want := []struct {
		id       string
		to       string
		value    int64
		typ      string
		callType string
		input    []byte
		output   []byte
	}{
		{"0x0100000000000000000000000000000000000000000000000000000000000001", strings.ToLower(addr1.Hex()), 7, "call", "call", []byte{}, []byte{}},
		{"0x0200000000000000000000000000000000000000000000000000000000000001", strings.ToLower(addr1.Hex()), 0, "create", "", code, code},
		{"0x0100000000000000000000000000000000000000000000000000000000000002", strings.ToLower(addr2.Hex()), 9, "call", "call", []byte{}, []byte{}},
		{"0x030000000000000000000000eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", 0, "create", "", []byte{}, []byte{}},
	}

	if len(txs) != len(want) || len(traces) != len(want) {
		t.Fatalf("got txs=%d traces=%d, want %d each", len(txs), len(traces), len(want))
	}
	if len(state) != 3 || state[addr1].Balance.Cmp(balanceBefore) != 0 || !bytes.Equal(state[addr1].Code, codeBefore) {
		t.Fatal("builder modified its genesis state input")
	}
	if txs[0].To != txs[1].To || txs[0].ID == txs[1].ID || !strings.HasPrefix(txs[0].ID, "0x01") || !strings.HasPrefix(txs[1].ID, "0x02") {
		t.Fatal("balance and code records for one address must use distinct kind ids")
	}
	for i, expected := range want {
		tx := txs[i]
		trace := traces[i]
		if tx.ID != expected.id || tx.To != expected.to || tx.TransactionIndex != int64(i) {
			t.Errorf("tx[%d] = {id:%s to:%s idx:%d}, want {id:%s to:%s idx:%d}", i, tx.ID, tx.To, tx.TransactionIndex, expected.id, expected.to, i)
		}
		if tx.From != (common.Address{}).Hex() || !tx.Status || tx.Value == nil || (*big.Int)(tx.Value).Int64() != expected.value || !bytes.Equal(tx.Input, expected.input) {
			t.Errorf("tx[%d] has unexpected synthetic transaction fields", i)
		}
		for name, value := range map[string]*big.Int{
			"gas": tx.Gas, "gas_price": tx.GasPrice, "gas_used": tx.GasUsed,
			"gas_fee_cap": tx.GasFeeCap, "gas_tip_cap": tx.GasTipCap, "nonce": tx.Nonce,
		} {
			if value == nil || value.Sign() != 0 {
				t.Errorf("tx[%d].%s = %v, want zero", i, name, value)
			}
		}
		if trace.ID != util.ToHash([]string{tx.ID, "", "0"}) || trace.TxID != tx.ID {
			t.Errorf("trace[%d] is not linked to tx %s", i, tx.ID)
		}
		if trace.From != tx.From || trace.To != expected.to || trace.Value == nil || (*big.Int)(trace.Value).Int64() != expected.value {
			t.Errorf("trace[%d] has unexpected address or value fields", i)
		}
		if trace.CallCreateType != expected.typ || trace.CallType != expected.callType || !bytes.Equal(trace.Input, expected.input) || !bytes.Equal(trace.Output, expected.output) {
			t.Errorf("trace[%d] has unexpected call fields", i)
		}
		if trace.Gas == nil || trace.Gas.Sign() != 0 || trace.GasUsed == nil || trace.GasUsed.Sign() != 0 || trace.ParentTraceID != "" || trace.PosInParentTrace != 0 || trace.SelfStorageChange || trace.StorageChange || trace.Subtraces != 0 || trace.TraceAddress == nil || len(trace.TraceAddress) != 0 {
			t.Errorf("trace[%d] has unexpected root trace fields", i)
		}
	}
}

func TestBuildGenesisSyntheticTransactionsDeterministic(t *testing.T) {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	account1 := types.Account{Balance: big.NewInt(1), Code: []byte{0x01}}
	account2 := types.Account{Balance: big.NewInt(2), Code: []byte{0x02}}

	forward := make(types.GenesisAlloc)
	forward[addr1] = account1
	forward[addr2] = account2
	reverse := make(types.GenesisAlloc)
	reverse[addr2] = account2
	reverse[addr1] = account1

	forwardTxs, forwardTraces := BuildGenesisSyntheticTransactions(forward)
	reverseTxs, reverseTraces := BuildGenesisSyntheticTransactions(reverse)
	if !reflect.DeepEqual(forwardTxs, reverseTxs) || !reflect.DeepEqual(forwardTraces, reverseTraces) {
		t.Fatal("genesis synthetic output depends on map insertion order")
	}
}

func TestBuildGenesisSyntheticTransactionsEmptyState(t *testing.T) {
	txs, traces := BuildGenesisSyntheticTransactions(types.GenesisAlloc{})
	wantID := "0x030000000000000000000000eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if len(txs) != 1 || len(traces) != 1 {
		t.Fatalf("got txs=%d traces=%d, want one native token record each", len(txs), len(traces))
	}
	if txs[0].ID != wantID || traces[0].TxID != wantID || txs[0].TransactionIndex != 0 || traces[0].CallCreateType != "create" {
		t.Fatal("empty genesis state did not produce the expected native token record")
	}
}

func TestBuildGenesisSyntheticTransactionsNilState(t *testing.T) {
	txs, traces := BuildGenesisSyntheticTransactions(nil)
	if len(txs) != 0 || len(traces) != 0 {
		t.Fatalf("nil genesis state produced txs=%d traces=%d, want none", len(txs), len(traces))
	}
}
