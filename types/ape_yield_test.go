package types

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

type apeYieldFixture struct {
	YieldAddress     common.Address `json:"yieldAddress"`
	YieldAddressHash common.Hash    `json:"yieldAddressHash"`
	Accounts         []struct {
		Address     common.Address `json:"address"`
		AddressHash common.Hash    `json:"addressHash"`
		Flags       uint8          `json:"flags"`
		Fixed       string         `json:"fixed"`
		Shares      string         `json:"shares"`
		Debt        string         `json:"debt"`
		Delegate    string         `json:"delegate"`
		Slots       []common.Hash  `json:"slots"`
		StorageKeys []common.Hash  `json:"storageKeys"`
		Values      []common.Hash  `json:"values"`
	} `json:"accounts"`
}

func TestApeYieldSlotsFixture(t *testing.T) {
	blob, err := os.ReadFile("testdata/ape_yield_slots.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture apeYieldFixture
	if err := json.Unmarshal(blob, &fixture); err != nil {
		t.Fatal(err)
	}
	if ApeYieldAddress != fixture.YieldAddress || ApeYieldAddressHash != fixture.YieldAddressHash {
		t.Fatalf("yield address %s/%s, fixture %s/%s", ApeYieldAddress, ApeYieldAddressHash, fixture.YieldAddress, fixture.YieldAddressHash)
	}
	for _, acc := range fixture.Accounts {
		if got := crypto.Keccak256Hash(acc.Address.Bytes()); got != acc.AddressHash {
			t.Fatalf("address hash %s, fixture %s", got, acc.AddressHash)
		}
		yield, err := ParseApeYield(acc.Flags, acc.Fixed, acc.Shares, acc.Debt, acc.Delegate)
		if err != nil {
			t.Fatal(err)
		}
		values := yield.Values()
		for i := 0; i < ApeYieldSlotCount; i++ {
			if got := ApeYieldSlot(acc.AddressHash, i); got != acc.Slots[i] {
				t.Errorf("slot %d: %s, fixture %s", i, got, acc.Slots[i])
			}
			if got := ApeYieldStorageKey(acc.AddressHash, i); got != acc.StorageKeys[i] {
				t.Errorf("storage key %d: %s, fixture %s", i, got, acc.StorageKeys[i])
			}
			if got := common.Hash(values[i].Bytes32()); got != acc.Values[i] {
				t.Errorf("value %d: %s, fixture %s", i, got, acc.Values[i])
			}
		}
	}
}

func apeWrites(changes []ApeYieldChange) map[common.Hash]uint64 {
	out := make(map[common.Hash]uint64)
	if diff := ApeYieldStorageDiff(changes); diff != nil {
		for _, pair := range diff.Values {
			out[pair.Index] = pair.Value.Uint64()
		}
	}
	return out
}

func TestApeYieldStorageDiff(t *testing.T) {
	addrHash := common.HexToHash("0x01")
	pre := ApeYieldAccount{Fixed: uint256.NewInt(5), Shares: uint256.NewInt(10), Debt: uint256.NewInt(0)}
	post := ApeYieldAccount{Fixed: uint256.NewInt(5), Shares: uint256.NewInt(11), Debt: uint256.NewInt(0)}
	key := func(i int) common.Hash { return ApeYieldStorageKey(addrHash, i) }

	// Only the changed slot is written.
	if got := apeWrites([]ApeYieldChange{{AddrHash: addrHash, Pre: &pre, Post: &post}}); len(got) != 1 || got[key(2)] != 11 {
		t.Fatalf("changed slot: %v", got)
	}
	// Unknown previous value: every slot is written, zeros included.
	if got := apeWrites([]ApeYieldChange{{AddrHash: addrHash, Post: &post}}); len(got) != 4 || got[key(0)] != 0 || got[key(1)] != 5 {
		t.Fatalf("unknown pre: %v", got)
	}
	// Deletion clears the non-zero slots.
	if got := apeWrites([]ApeYieldChange{{AddrHash: addrHash, Pre: &pre}}); len(got) != 2 || got[key(1)] != 0 || got[key(2)] != 0 {
		t.Fatalf("deletion: %v", got)
	}
	// A new account from zero writes only its non-zero slots.
	if got := apeWrites([]ApeYieldChange{{AddrHash: addrHash, Pre: &ApeYieldAccount{}, Post: &post}}); len(got) != 2 {
		t.Fatalf("new account: %v", got)
	}
	if diff := ApeYieldStorageDiff([]ApeYieldChange{{AddrHash: addrHash, Pre: &pre, Post: &pre}}); diff != nil {
		t.Fatalf("unchanged account writes %v", diff.Values)
	}
}

func apeSlim(t *testing.T, flags uint8, fixed, shares, debt uint64, delegate common.Address) []byte {
	t.Helper()
	blob, err := rlp.EncodeToBytes(&ApeSlimAccount{
		Flags: flags, Fixed: uint256.NewInt(fixed), Shares: uint256.NewInt(shares), Debt: uint256.NewInt(debt), Delegate: delegate,
	})
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func TestApeYieldChangesFromCommit(t *testing.T) {
	var (
		delegator = common.HexToAddress("0xa1")
		delegate  = common.HexToAddress("0xd1")
		gone      = common.HexToAddress("0xc1")
		created   = common.HexToAddress("0xe1")
		hash      = func(a common.Address) common.Hash { return crypto.Keccak256Hash(a.Bytes()) }
	)
	accounts := map[common.Hash][]byte{
		hash(delegator): apeSlim(t, ApeYieldDelegated, 150, 0, 0, delegate),
		hash(delegate):  apeSlim(t, ApeYieldAutomatic, 3, 40, 150, common.Address{}),
		hash(created):   apeSlim(t, ApeYieldDisabled, 9, 0, 0, common.Address{}),
	}
	origin := map[common.Address][]byte{
		delegator: apeSlim(t, ApeYieldDelegated, 100, 0, 0, delegate),
		delegate:  apeSlim(t, ApeYieldAutomatic, 3, 35, 100, common.Address{}),
		gone:      apeSlim(t, ApeYieldAutomatic, 1, 2, 0, common.Address{}),
		created:   nil,
	}
	destructs := map[common.Hash]struct{}{hash(gone): {}}

	changes, err := ApeYieldChangesFromCommit(destructs, accounts, origin)
	if err != nil {
		t.Fatal(err)
	}
	got := apeWrites(changes)
	disabledSlot0 := ApeYieldAccount{Flags: ApeYieldDisabled}.Values()[0].Uint64()
	want := map[common.Hash]uint64{
		ApeYieldStorageKey(hash(delegator), 1): 150,
		ApeYieldStorageKey(hash(delegate), 2):  40,
		ApeYieldStorageKey(hash(delegate), 3):  150,
		ApeYieldStorageKey(hash(gone), 1):      0,
		ApeYieldStorageKey(hash(gone), 2):      0,
		ApeYieldStorageKey(hash(created), 0):   disabledSlot0,
		ApeYieldStorageKey(hash(created), 1):   9,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d writes, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			t.Errorf("slot %s: got %d (present %v), want %d", k, g, ok, v)
		}
	}

	// Without origins every slot of every account is written.
	changes, err = ApeYieldChangesFromCommit(destructs, accounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(apeWrites(changes)); n != 4*4 {
		t.Fatalf("got %d writes without origins, want 16", n)
	}
}
