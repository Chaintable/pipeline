package tracer

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/XinFinOrg/XDPoSChain/common"
	"github.com/XinFinOrg/XDPoSChain/core/types"
)

func TestBuildGenesisBlockFile(t *testing.T) {
	address := common.HexToAddress("0x0000000000000000000000000000000000000088")
	alloc := ptypes.GenesisAlloc{
		address:           {Balance: big.NewInt(7), Code: []byte{0x60, 0x00}, Storage: map[common.Hash]common.Hash{{}: {1}}},
		common.Address{1}: {Balance: nil},
	}
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(0), Time: big.NewInt(1), Difficulty: big.NewInt(1)})
	bf := BuildGenesisBlockFile(block, alloc)
	if len(bf.Txs) != 3 || len(bf.Traces) != 3 || !reflect.DeepEqual(bf.StorageContracts, []string{"0x0000000000000000000000000000000000000088"}) {
		t.Fatalf("wrong genesis counts: %+v", bf.Validation())
	}
	wantIDs := []string{
		"0x0100000000000000000000000000000000000000000000000000000000000088",
		"0x0200000000000000000000000000000000000000000000000000000000000088",
		"0x030000000000000000000000eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	}
	for i, tx := range bf.Txs {
		trace := bf.Traces[i]
		if tx.ID != wantIDs[i] || tx.TransactionIndex != int64(i) || trace.TxID != tx.ID || trace.ID != fmt.Sprintf("%x", md5.Sum([]byte(tx.ID+"0"))) {
			t.Fatalf("invalid record pair %d: %+v %+v", i, tx, trace)
		}
	}
	if (*big.Int)(bf.Txs[0].Value).Cmp(big.NewInt(7)) != 0 || bf.Traces[0].CallCreateType != "call" || !bytes.Equal(bf.Traces[1].Output, alloc[address].Code) || bf.Traces[2].CallCreateType != "create" {
		t.Fatal("incorrect balance, code or native record")
	}
	if bf.Validation().StorageContractsCount != 1 {
		t.Fatal("missing storage contract count")
	}
	for i := 0; i < 20; i++ {
		again := BuildGenesisBlockFile(block, alloc)
		if !reflect.DeepEqual(again.Txs, bf.Txs) || !reflect.DeepEqual(again.Traces, bf.Traces) || !reflect.DeepEqual(again.StorageContracts, bf.StorageContracts) {
			t.Fatal("nondeterministic genesis records")
		}
	}
	// RPC callers must work without initializing global uploaders or failover.
	empty := BuildGenesisBlockFile(block, nil)
	if len(empty.Txs) != 1 || len(empty.Traces) != 1 || empty.Txs[0].ID != wantIDs[2] {
		t.Fatal("empty alloc must still include the native placeholder")
	}
	encoded, err := json.Marshal(empty)
	if err != nil || bytes.Contains(encoded, []byte(":null")) {
		t.Fatalf("empty genesis collections should serialize as arrays: %s (%v)", encoded, err)
	}
}
