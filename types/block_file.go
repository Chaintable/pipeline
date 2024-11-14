package types

import (
	"crypto/sha1"
	"encoding/hex"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type BlockFile struct {
	Block            Block             `json:"block"`
	Txs              []Transaction     `json:"txs"`
	Events           []Event           `json:"events"`
	Traces           []Trace           `json:"traces"`
	SpecialTransfers []SpecialTransfer `json:"special_transfers"`
}

func (bf *BlockFile) ValidationHash() int64 {
	var ids []string

	// Collect all IDs
	ids = append(ids, bf.Block.ID) // assuming Block has an ID field
	for _, tx := range bf.Txs {
		ids = append(ids, tx.ID)
	}
	for _, event := range bf.Events {
		ids = append(ids, event.ID)
	}
	for _, trace := range bf.Traces {
		ids = append(ids, trace.ID)
	}
	for _, withdrawal := range bf.SpecialTransfers {
		ids = append(ids, withdrawal.ID)
	}

	return CalcValidationHash(ids)
}

func CalcValidationHash(ids []string) int64 {
	sha1Sum := big.NewInt(0)
	for _, each := range ids {
		h := sha1.New()
		h.Write([]byte(each))
		hash := hex.EncodeToString(h.Sum(nil))
		hashInt, err := hexutil.DecodeBig("0x" + hash)
		if err != nil {
			panic(err)
		}
		sha1Sum.Add(sha1Sum, hashInt)
	}

	sha1SumStr := sha1Sum.String()
	last8Digits := sha1SumStr[len(sha1SumStr)-8:]
	validationHash, _ := strconv.ParseInt(last8Digits, 10, 64)
	return validationHash
}
