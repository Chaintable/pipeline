package types

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

type BlockFile struct {
	Block                    Block                      `json:"block"`
	Txs                      []Transaction              `json:"txs"`
	Events                   []Event                    `json:"events"`
	Traces                   []Trace                    `json:"traces"`
	MinerNativeTransfers     []MinerNativeRransfer      `json:"miner_native_transfers"`
	WithdrawalNativeTransfer []WithdrawalNativeTransfer `json:"withdrawal_native_transfer"`
}

func (bf *BlockFile) ValidationHash() int {
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
	for _, transfer := range bf.MinerNativeTransfers {
		ids = append(ids, transfer.ID)
	}
	for _, withdrawal := range bf.WithdrawalNativeTransfer {
		ids = append(ids, withdrawal.ID)
	}

	// Calculate the SHA-256 hash sum
	var sha256Sum int
	for _, id := range ids {
		hash := sha256.Sum256([]byte(id))
		hashInt, _ := strconv.ParseInt(hex.EncodeToString(hash[:]), 16, 64)
		sha256Sum += int(hashInt)
	}

	// Get the last four digits
	validationHash, _ := strconv.Atoi(strconv.Itoa(sha256Sum)[len(strconv.Itoa(sha256Sum))-4:])
	return validationHash
}
