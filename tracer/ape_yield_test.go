package tracer

import (
	"testing"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

func TestStateUpdateToStateDiffApeYield(t *testing.T) {
	addr := common.HexToAddress("0xa1")
	addrHash := crypto.Keccak256Hash(addr.Bytes())
	encode := func(acct *ptypes.ApeSlimAccount) []byte {
		blob, err := rlp.EncodeToBytes(acct)
		if err != nil {
			t.Fatal(err)
		}
		return blob
	}
	post := encode(&ptypes.ApeSlimAccount{Nonce: 2, Fixed: uint256.NewInt(5), Shares: uint256.NewInt(11), Debt: new(uint256.Int)})
	pre := encode(&ptypes.ApeSlimAccount{Nonce: 1, Fixed: uint256.NewInt(5), Shares: uint256.NewInt(10), Debt: new(uint256.Int)})
	accounts := map[common.Hash][]byte{addrHash: post}

	yieldWrites := func(origin map[common.Address][]byte) map[common.Hash]uint64 {
		diff := stateUpdateToStateDiff(common.Hash{}, common.Hash{1}, nil, accounts, origin, nil, nil, nil)
		if len(diff.NewAccounts) != 1 {
			t.Fatalf("unexpected accounts %+v", diff.NewAccounts)
		}
		if acc := diff.NewAccounts[0]; !acc.Balance.IsZero() || acc.Nonce != 2 || acc.CodeHash != types.EmptyCodeHash {
			t.Fatalf("unexpected account %+v", acc)
		}
		writes := make(map[common.Hash]uint64)
		for _, storage := range diff.StorageDiff {
			if storage.Address != ptypes.ApeYieldAddressHash {
				t.Fatalf("unexpected storage account %x", storage.Address)
			}
			for _, pair := range storage.Values {
				writes[pair.Index] = pair.Value.Uint64()
			}
		}
		return writes
	}

	// Live commit: only the changed shares slot.
	got := yieldWrites(map[common.Address][]byte{addr: pre})
	if len(got) != 1 || got[ptypes.ApeYieldStorageKey(addrHash, 2)] != 11 {
		t.Fatalf("live commit writes %v", got)
	}
	// debank_block RPC path, no origins: all four slots.
	got = yieldWrites(nil)
	if len(got) != ptypes.ApeYieldSlotCount || got[ptypes.ApeYieldStorageKey(addrHash, 1)] != 5 {
		t.Fatalf("rpc path writes %v", got)
	}
}
