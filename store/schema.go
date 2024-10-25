package store

import (
	"encoding/binary"
)

var (
	// BlockNumber+BlockHash+BlockEventCountPrefix -> TotalEventCount
	BlockEventCountPrefix = []byte("c")
	// BlockNumber+BlockHash+DiffPrefix -> Diff
	DiffPrefix = []byte("d")
	// BlockNumber+BlockHash+TracePrefix+TxHash -> []CallFrame
	TracePrefix = []byte("t")
	// BlockNumber+BlockHash+EventIndexPrefix -> []EventPosition
	EventIndexPrefix = []byte("e")
)

func EncodeNumber(blockNumber uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, blockNumber)
	return b
}

func DecodeNumber(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}
