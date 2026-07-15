package types

import (
	"bytes"
	"encoding/hex"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

func TestBlastBlockStorageDiffRLP(t *testing.T) {
	diff := BlastBlockStorageDiff{
		Hash:       common.HexToHash("0x01"),
		ParentHash: common.HexToHash("0x02"),
		NewAccounts: []BlastNewAccount{{
			Address:   common.HexToHash("0x03"),
			Nonce:     7,
			Flags:     2,
			Fixed:     uint256.NewInt(11),
			Shares:    uint256.NewInt(13),
			Remainder: uint256.NewInt(17),
			CodeHash:  common.HexToHash("0x04"),
		}},
		DeletedAccounts: []common.Hash{common.HexToHash("0x05")},
		StorageDiff: []AccountStorageDiff{{
			Address: common.HexToHash("0x06"),
			Values: []IndexValuePair{{
				Index: common.HexToHash("0x07"),
				Value: uint256.NewInt(19),
			}},
		}},
		NewCodes: []NewCode{{
			CodeHash: common.HexToHash("0x08"),
			Code:     []byte{0xde, 0xad, 0xbe, 0xef},
		}},
	}

	encoded, err := rlp.EncodeToBytes(&diff)
	if err != nil {
		t.Fatalf("encode Blast state diff: %v", err)
	}
	fixture, err := os.ReadFile("testdata/blast_state_diff.rlp.hex")
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	want, err := hex.DecodeString(strings.TrimSpace(string(fixture)))
	if err != nil {
		t.Fatalf("decode golden fixture: %v", err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("unexpected RLP\nwant: %x\n got: %x", want, encoded)
	}

	var decoded BlastBlockStorageDiff
	if err := rlp.DecodeBytes(encoded, &decoded); err != nil {
		t.Fatalf("decode Blast state diff: %v", err)
	}
	if !reflect.DeepEqual(decoded, diff) {
		t.Fatalf("round trip mismatch\nwant: %#v\n got: %#v", diff, decoded)
	}
}
