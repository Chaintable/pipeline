package store

import (
	"encoding/binary"
)

var (
	// BlockHash+BlockEventCountPrefix -> TotalEventCount
	BlockEventCountPrefix = []byte("c")
)

func EncodeNumber(blockNumber uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, blockNumber)
	return b
}

func DecodeNumber(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}
