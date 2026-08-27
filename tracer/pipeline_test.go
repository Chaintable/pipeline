package tracer

import (
	"math/big"
	"strings"
	"testing"

	"github.com/morph-l2/go-ethereum/common"
	statesnapshot "github.com/morph-l2/go-ethereum/core/state/snapshot"
	"github.com/morph-l2/go-ethereum/core/types"
	"github.com/morph-l2/go-ethereum/crypto"
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

func TestStateUpdateToStateDiffRejectsMalformedStorage(t *testing.T) {
	addressHash := common.HexToHash("0x01")
	index := common.HexToHash("0x02")

	_, err := stateUpdateToStateDiff(
		common.HexToHash("0x01"),
		common.HexToHash("0x02"),
		nil,
		nil,
		nil,
		map[common.Hash]map[common.Hash][]byte{
			addressHash: {index: {0xff}},
		},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("stateUpdateToStateDiff() error = nil, want malformed storage error")
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
