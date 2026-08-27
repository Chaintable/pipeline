package tracer

import (
	"math/big"
	"strings"
	"testing"

	"github.com/morph-l2/go-ethereum/common"
	statesnapshot "github.com/morph-l2/go-ethereum/core/state/snapshot"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/crypto"
	"github.com/morph-l2/go-ethereum/rlp"
)

func TestStateUpdateToStateDiffIncludesSlimEOA(t *testing.T) {
	addressHash := common.HexToHash("0x5736707eda95b86f67280e719e4310e50f3914541940f6edfcb0709da21a24c9")
	balance := big.NewInt(194853220000000000)
	emptyCodeHash := crypto.Keccak256Hash(nil)
	account := statesnapshot.SlimAccountRLP(
		0,
		balance,
		types.EmptyRootHash,
		emptyCodeHash.Bytes(),
		nil,
		0,
	)

	diff, err := stateUpdateToStateDiff(
		common.HexToHash("0x01"),
		common.HexToHash("0x02"),
		nil,
		map[common.Hash][]byte{addressHash: account},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("stateUpdateToStateDiff() error = %v", err)
	}

	if len(diff.NewAccounts) != 1 {
		t.Fatalf("new accounts length = %d, want 1", len(diff.NewAccounts))
	}
	got := diff.NewAccounts[0]
	if got.Address != addressHash {
		t.Fatalf("address = %s, want %s", got.Address, addressHash)
	}
	if got.Balance.ToBig().Cmp(balance) != 0 {
		t.Fatalf("balance = %s, want %s", got.Balance, balance)
	}
	if got.Nonce != 0 {
		t.Fatalf("nonce = %d, want 0", got.Nonce)
	}
	if got.CodeHash != emptyCodeHash {
		t.Fatalf("code hash = %s, want %s", got.CodeHash, emptyCodeHash)
	}
}

func TestStateUpdateToStateDiffRejectsMalformedAccount(t *testing.T) {
	addressHash := common.HexToHash("0x01")

	_, err := stateUpdateToStateDiff(
		common.HexToHash("0x01"),
		common.HexToHash("0x02"),
		nil,
		map[common.Hash][]byte{addressHash: {0x01}},
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("stateUpdateToStateDiff() error = nil, want malformed account error")
	}
	if !strings.Contains(err.Error(), addressHash.String()) {
		t.Fatalf("stateUpdateToStateDiff() error = %q, want account hash", err)
	}
}

func TestStateUpdateToStateDiffRejectsInvalidAccountFields(t *testing.T) {
	overflowBalance := new(big.Int).Lsh(big.NewInt(1), 256)
	tests := map[string]statesnapshot.Account{
		"short storage root": {
			Balance:        big.NewInt(1),
			Root:           []byte{0x01},
			KeccakCodeHash: crypto.Keccak256(nil),
		},
		"short code hash": {
			Balance:        big.NewInt(1),
			Root:           types.EmptyRootHash.Bytes(),
			KeccakCodeHash: []byte{0x01},
		},
		"overflow balance": {
			Balance:        overflowBalance,
			Root:           types.EmptyRootHash.Bytes(),
			KeccakCodeHash: crypto.Keccak256(nil),
		},
	}
	for name, account := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := rlp.EncodeToBytes(account)
			if err != nil {
				t.Fatalf("encode account: %v", err)
			}
			_, err = stateUpdateToStateDiff(
				common.HexToHash("0x01"),
				common.HexToHash("0x02"),
				nil,
				map[common.Hash][]byte{common.HexToHash("0x03"): encoded},
				nil,
				nil,
				nil,
				nil,
			)
			if err == nil {
				t.Fatal("stateUpdateToStateDiff() error = nil, want invalid account error")
			}
		})
	}
}

func TestStateUpdateToStateDiffRejectsMalformedStorage(t *testing.T) {
	addressHash := common.HexToHash("0x01")
	index := common.HexToHash("0x02")
	oversized, err := rlp.EncodeToBytes(make([]byte, common.HashLength+1))
	if err != nil {
		t.Fatalf("encode oversized storage: %v", err)
	}
	tests := map[string][]byte{
		"invalid RLP":     {0xff},
		"RLP list":        {0xc1, 0x01},
		"trailing bytes":  {0x01, 0x02},
		"oversized value": oversized,
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := stateUpdateToStateDiff(
				common.HexToHash("0x01"),
				common.HexToHash("0x02"),
				nil,
				nil,
				nil,
				map[common.Hash]map[common.Hash][]byte{
					addressHash: {index: value},
				},
				nil,
				nil,
			)
			if err == nil {
				t.Fatal("stateUpdateToStateDiff() error = nil, want malformed storage error")
			}
			if !strings.Contains(err.Error(), addressHash.String()) || !strings.Contains(err.Error(), index.String()) {
				t.Fatalf("stateUpdateToStateDiff() error = %q, want account and slot hashes", err)
			}
		})
	}
}

func TestStateUpdateToStateDiffIncludesContractAccount(t *testing.T) {
	addressHash := common.HexToHash("0x01")
	storageRoot := common.HexToHash("0x02")
	codeHash := crypto.Keccak256Hash([]byte{0x60, 0x00})
	account := statesnapshot.SlimAccountRLP(
		7,
		big.NewInt(0),
		storageRoot,
		codeHash.Bytes(),
		common.HexToHash("0x03").Bytes(),
		2,
	)

	diff, err := stateUpdateToStateDiff(
		common.HexToHash("0x04"),
		common.HexToHash("0x05"),
		nil,
		map[common.Hash][]byte{addressHash: account},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("stateUpdateToStateDiff() error = %v", err)
	}
	if len(diff.NewAccounts) != 1 {
		t.Fatalf("new accounts length = %d, want 1", len(diff.NewAccounts))
	}
	got := diff.NewAccounts[0]
	if got.Address != addressHash || got.Nonce != 7 || !got.Balance.IsZero() || got.CodeHash != codeHash {
		t.Fatalf("contract account = %+v, want address %s nonce 7 balance 0 code hash %s", got, addressHash, codeHash)
	}
}

func TestStateUpdateToStateDiffIncludesStorageCodeAndDeletion(t *testing.T) {
	accountHash := common.HexToHash("0x01")
	deletedHash := common.HexToHash("0x02")
	setIndex := common.HexToHash("0x03")
	clearIndex := common.HexToHash("0x04")
	setValue, err := rlp.EncodeToBytes([]byte{0x2a})
	if err != nil {
		t.Fatalf("encode storage value: %v", err)
	}
	code := []byte{0x60, 0x00}
	codeHash := crypto.Keccak256Hash(code)

	diff, err := stateUpdateToStateDiff(
		common.Hash{},
		common.Hash{},
		map[common.Hash]struct{}{deletedHash: {}},
		nil,
		nil,
		map[common.Hash]map[common.Hash][]byte{
			accountHash: {
				setIndex:   setValue,
				clearIndex: nil,
			},
		},
		nil,
		map[common.Hash][]byte{codeHash: code},
	)
	if err != nil {
		t.Fatalf("stateUpdateToStateDiff() error = %v", err)
	}
	if diff.ParentHash != types.EmptyRootHash || diff.Hash != types.EmptyRootHash {
		t.Fatalf("roots = (%s, %s), want empty root", diff.ParentHash, diff.Hash)
	}
	if len(diff.DeletedAccounts) != 1 || diff.DeletedAccounts[0] != deletedHash {
		t.Fatalf("deleted accounts = %v, want %s", diff.DeletedAccounts, deletedHash)
	}
	if len(diff.NewCodes) != 1 || diff.NewCodes[0].CodeHash != codeHash || string(diff.NewCodes[0].Code) != string(code) {
		t.Fatalf("new codes = %+v, want hash %s code %x", diff.NewCodes, codeHash, code)
	}
	if len(diff.StorageDiff) != 1 || diff.StorageDiff[0].Address != accountHash {
		t.Fatalf("storage diff = %+v, want account %s", diff.StorageDiff, accountHash)
	}
	values := make(map[common.Hash]*big.Int)
	for _, pair := range diff.StorageDiff[0].Values {
		values[pair.Index] = pair.Value.ToBig()
	}
	if got := values[setIndex]; got == nil || got.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("set storage value = %v, want 42", got)
	}
	if got := values[clearIndex]; got == nil || got.Sign() != 0 {
		t.Fatalf("cleared storage value = %v, want 0", got)
	}
}

func TestStateUpdateToStateDiffUsesKeccakCodeHash(t *testing.T) {
	addressHash := common.HexToHash("0x01")
	keccakCodeHash := common.HexToHash("0x1111")
	poseidonCodeHash := common.HexToHash("0x2222")
	account := statesnapshot.SlimAccountRLP(
		7,
		big.NewInt(1),
		types.EmptyRootHash,
		keccakCodeHash.Bytes(),
		poseidonCodeHash.Bytes(),
		42,
	)

	diff, err := stateUpdateToStateDiff(
		common.HexToHash("0x01"),
		common.HexToHash("0x02"),
		nil,
		map[common.Hash][]byte{addressHash: account},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("stateUpdateToStateDiff() error = %v", err)
	}
	if got := diff.NewAccounts[0].CodeHash; got != keccakCodeHash {
		t.Fatalf("code hash = %s, want Keccak code hash %s", got, keccakCodeHash)
	}
}

func TestRPCTracerGetOutPutPropagatesStateDiffError(t *testing.T) {
	tracer := &RPCTracer{currentBlock: &RPCBlockContext{}}
	addressHash := common.HexToHash("0x01")

	_, err := tracer.GetOutPut(
		common.HexToHash("0x01"),
		common.HexToHash("0x02"),
		nil,
		map[common.Hash][]byte{addressHash: {0x01}},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("GetOutPut() error = nil, want state diff error")
	}
	if !strings.Contains(err.Error(), "decode slim account") {
		t.Fatalf("GetOutPut() error = %q, want decode slim account context", err)
	}
}
