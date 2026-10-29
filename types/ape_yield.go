package types

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

// ApeChain accounts store Flags, Fixed, Shares, Debt and Delegate instead of a
// balance. State diffs keep the account shape of other chains: Balance is zero
// and the raw yield fields are written as storage of a reserved account. RPC
// nodes resolve the balance from those slots and the ArbOS share price of the
// same block.
//
// Logical slot i of an account is keccak256(addrHash ++ uint256(i)). The state
// diff keys it by keccak256(slot), like any other storage slot.
//
//	slot 0: flags << 248 | delegate (low 20 bytes)
//	slot 1: fixed
//	slot 2: shares
//	slot 3: debt
var (
	// ApeYieldAddress is "\xa4\xb0" ++ "ape-yield-state" ++ "\x00\x00\x00", outside
	// the ArbOS (0xA4B05FFF...) and precompile ranges.
	ApeYieldAddress     = common.HexToAddress("0xa4b06170652d7969656c642d7374617465000000")
	ApeYieldAddressHash = crypto.Keccak256Hash(ApeYieldAddress.Bytes())
)

const ApeYieldSlotCount = 4

// Yield modes, as in the ApeChain go-ethereum fork.
const (
	ApeYieldAutomatic uint8 = iota
	ApeYieldDisabled
	ApeYieldDelegated
)

// ApeSlimAccount is the ApeChain account in slim RLP encoding, as written by
// the ApeChain go-ethereum fork (types.SlimAccountRLP).
type ApeSlimAccount struct {
	Nonce    uint64
	Flags    uint8
	Fixed    *uint256.Int
	Shares   *uint256.Int
	Debt     *uint256.Int
	Delegate common.Address
	Root     []byte // nil if the storage root is empty
	CodeHash []byte // nil if the code hash is empty
}

// DecodeApeSlimAccount decodes an ApeChain account in slim RLP encoding.
func DecodeApeSlimAccount(blob []byte) (*ApeSlimAccount, error) {
	var acct ApeSlimAccount
	if err := rlp.DecodeBytes(blob, &acct); err != nil {
		return nil, err
	}
	return &acct, nil
}

// CodeHashOrEmpty returns the code hash, the empty code hash when unset.
func (a *ApeSlimAccount) CodeHashOrEmpty() common.Hash {
	if len(a.CodeHash) == 0 {
		return types.EmptyCodeHash
	}
	return common.BytesToHash(a.CodeHash)
}

// Yield returns the yield fields; nil values read as zero.
func (a *ApeSlimAccount) Yield() ApeYieldAccount {
	return ApeYieldAccount{
		Flags:    a.Flags,
		Fixed:    orZero(a.Fixed),
		Shares:   orZero(a.Shares),
		Debt:     orZero(a.Debt),
		Delegate: a.Delegate,
	}
}

// ApeYieldAccount holds the yield fields of an ApeChain account.
type ApeYieldAccount struct {
	Flags    uint8
	Fixed    *uint256.Int
	Shares   *uint256.Int
	Debt     *uint256.Int
	Delegate common.Address
}

// ParseApeYield builds the yield fields from their decimal and hex string forms.
func ParseApeYield(flags uint8, fixed, shares, debt, delegate string) (ApeYieldAccount, error) {
	var (
		y   = ApeYieldAccount{Flags: flags, Delegate: common.HexToAddress(delegate)}
		err error
	)
	if y.Fixed, err = uint256.FromDecimal(fixed); err != nil {
		return y, fmt.Errorf("invalid fixed %q: %w", fixed, err)
	}
	if y.Shares, err = uint256.FromDecimal(shares); err != nil {
		return y, fmt.Errorf("invalid shares %q: %w", shares, err)
	}
	if y.Debt, err = uint256.FromDecimal(debt); err != nil {
		return y, fmt.Errorf("invalid debt %q: %w", debt, err)
	}
	return y, nil
}

// Values returns the four slot values in slot order.
func (y ApeYieldAccount) Values() [ApeYieldSlotCount]*uint256.Int {
	var packed [32]byte
	packed[0] = y.Flags
	copy(packed[12:], y.Delegate.Bytes())
	return [ApeYieldSlotCount]*uint256.Int{
		new(uint256.Int).SetBytes32(packed[:]),
		orZero(y.Fixed).Clone(),
		orZero(y.Shares).Clone(),
		orZero(y.Debt).Clone(),
	}
}

// ApeYieldSlot returns logical slot i of the account with the given address hash.
func ApeYieldSlot(addrHash common.Hash, i int) common.Hash {
	var index [32]byte
	index[31] = byte(i)
	return crypto.Keccak256Hash(addrHash.Bytes(), index[:])
}

// ApeYieldStorageKey returns the state diff key of logical slot i.
func ApeYieldStorageKey(addrHash common.Hash, i int) common.Hash {
	return crypto.Keccak256Hash(ApeYieldSlot(addrHash, i).Bytes())
}

// ApeYieldChange is the yield state of one account in a block. A nil Post means
// the account was deleted. A nil Pre means every slot is written, as when the
// previous value is unknown.
type ApeYieldChange struct {
	AddrHash common.Hash
	Pre      *ApeYieldAccount
	Post     *ApeYieldAccount
}

// ApeYieldStorageDiff returns the writes to the reserved yield account, sorted
// by key, or nil when no slot is written. With a known Pre, only slots whose
// value changed are written. A zero value deletes the slot.
func ApeYieldStorageDiff(changes []ApeYieldChange) *AccountStorageDiff {
	var values []IndexValuePair
	for _, c := range changes {
		var post [ApeYieldSlotCount]*uint256.Int
		if c.Post != nil {
			post = c.Post.Values()
		}
		var pre [ApeYieldSlotCount]*uint256.Int
		if c.Pre != nil {
			pre = c.Pre.Values()
		}
		for i := 0; i < ApeYieldSlotCount; i++ {
			value := orZero(post[i])
			if c.Pre != nil && value.Eq(pre[i]) {
				continue
			}
			values = append(values, IndexValuePair{Index: ApeYieldStorageKey(c.AddrHash, i), Value: value.Clone()})
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return bytes.Compare(values[i].Index[:], values[j].Index[:]) < 0 })
	return &AccountStorageDiff{Address: ApeYieldAddressHash, Values: values}
}

// ApeYieldChangesFromCommit pairs committed accounts with their pre-block yield
// fields. accounts maps address hash to slim RLP, nil when deleted; destructs
// lists more deleted accounts. accountsOrigin maps address to the pre-block slim
// RLP, empty when the account did not exist. A nil accountsOrigin, or an account
// missing from it, makes every slot of that account written.
func ApeYieldChangesFromCommit(destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, accountsOrigin map[common.Address][]byte) ([]ApeYieldChange, error) {
	var origins map[common.Hash]*ApeYieldAccount
	if accountsOrigin != nil {
		origins = make(map[common.Hash]*ApeYieldAccount, len(accountsOrigin))
		for addr, blob := range accountsOrigin {
			pre := new(ApeYieldAccount)
			if len(blob) > 0 {
				acct, err := DecodeApeSlimAccount(blob)
				if err != nil {
					return nil, fmt.Errorf("decode origin account %s: %w", addr, err)
				}
				*pre = acct.Yield()
			}
			origins[crypto.Keccak256Hash(addr.Bytes())] = pre
		}
	}
	changes := make([]ApeYieldChange, 0, len(accounts)+len(destructs))
	for addrHash, blob := range accounts {
		change := ApeYieldChange{AddrHash: addrHash, Pre: origins[addrHash]}
		if blob != nil {
			acct, err := DecodeApeSlimAccount(blob)
			if err != nil {
				return nil, fmt.Errorf("decode account %x: %w", addrHash, err)
			}
			post := acct.Yield()
			change.Post = &post
		}
		changes = append(changes, change)
	}
	for addrHash := range destructs {
		if _, recreated := accounts[addrHash]; recreated {
			continue
		}
		changes = append(changes, ApeYieldChange{AddrHash: addrHash, Pre: origins[addrHash]})
	}
	return changes, nil
}

func orZero(v *uint256.Int) *uint256.Int {
	if v == nil {
		return new(uint256.Int)
	}
	return v
}
