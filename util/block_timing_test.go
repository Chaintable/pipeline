package util

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/segmentio/kafka-go"
)

func TestBlockFirstSeenHeadersRoundTrip(t *testing.T) {
	firstHash := common.HexToHash("0x01")
	secondHash := common.HexToHash("0x02")
	firstSeen := time.Date(2026, time.August, 10, 12, 0, 0, 123*int(time.Millisecond), time.UTC)
	secondSeen := firstSeen.Add(2 * time.Second)

	headers := EncodeBlockFirstSeenHeaders([]types.BlockContext{
		{Hash: firstHash, FirstSeenAtUnixMilli: firstSeen.UnixMilli()},
		{Hash: secondHash, FirstSeenAtUnixMilli: secondSeen.UnixMilli()},
		{Hash: common.HexToHash("0x03")},
	})
	if len(headers) != 2 {
		t.Fatalf("EncodeBlockFirstSeenHeaders returned %d headers, want 2", len(headers))
	}

	timings := DecodeBlockFirstSeenHeaders(headers)
	if got := timings[firstHash]; !got.Equal(firstSeen) {
		t.Fatalf("first timing = %v, want %v", got, firstSeen)
	}
	if got := timings[secondHash]; !got.Equal(secondSeen) {
		t.Fatalf("second timing = %v, want %v", got, secondSeen)
	}
}

func TestDecodeBlockFirstSeenHeadersUsesEarliestValidValue(t *testing.T) {
	hash := common.HexToHash("0x01")
	early := time.UnixMilli(1_700_000_000_000)
	late := early.Add(time.Second)
	headers := EncodeBlockFirstSeenHeaders([]types.BlockContext{
		{Hash: hash, FirstSeenAtUnixMilli: late.UnixMilli()},
		{Hash: hash, FirstSeenAtUnixMilli: early.UnixMilli()},
	})
	headers = append(headers,
		kafka.Header{Key: BlockFirstSeenHeaderKey, Value: []byte("malformed")},
		kafka.Header{Key: "unknown", Value: make([]byte, blockFirstSeenValueSize)},
	)

	if got := DecodeBlockFirstSeenHeaders(headers)[hash]; !got.Equal(early) {
		t.Fatalf("decoded timing = %v, want %v", got, early)
	}
}

func TestBlockFirstSeenTimingIsNotSerialized(t *testing.T) {
	block := types.BlockContext{
		Hash:                 common.HexToHash("0x01"),
		FirstSeenAtUnixMilli: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["firstSeenAtUnixMilli"]; ok {
		t.Fatal("first-seen timing leaked into the Kafka payload")
	}
}
