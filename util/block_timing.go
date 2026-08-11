package util

import (
	"encoding/binary"
	"time"

	"github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/segmentio/kafka-go"
)

const (
	BlockFirstSeenHeaderKey = "debank-block-first-seen-v1"
	blockFirstSeenValueSize = common.HashLength + 8
)

// EncodeBlockFirstSeenHeaders creates one timing header per new block. The
// payload remains unchanged, and missing timings are omitted.
func EncodeBlockFirstSeenHeaders(blocks []types.BlockContext) []kafka.Header {
	headers := make([]kafka.Header, 0, len(blocks))
	for _, block := range blocks {
		if block.FirstSeenAtUnixMilli <= 0 {
			continue
		}
		value := make([]byte, blockFirstSeenValueSize)
		copy(value, block.Hash[:])
		binary.BigEndian.PutUint64(value[common.HashLength:], uint64(block.FirstSeenAtUnixMilli))
		headers = append(headers, kafka.Header{Key: BlockFirstSeenHeaderKey, Value: value})
	}
	return headers
}

// DecodeBlockFirstSeenHeaders returns the earliest valid timestamp for each
// block hash. Unknown and malformed headers are ignored for compatibility.
func DecodeBlockFirstSeenHeaders(headers []kafka.Header) map[common.Hash]time.Time {
	timings := make(map[common.Hash]time.Time)
	for _, header := range headers {
		if header.Key != BlockFirstSeenHeaderKey || len(header.Value) != blockFirstSeenValueSize {
			continue
		}
		millis := int64(binary.BigEndian.Uint64(header.Value[common.HashLength:]))
		if millis <= 0 {
			continue
		}
		hash := common.BytesToHash(header.Value[:common.HashLength])
		seenAt := time.UnixMilli(millis)
		if current, ok := timings[hash]; !ok || seenAt.Before(current) {
			timings[hash] = seenAt
		}
	}
	return timings
}
