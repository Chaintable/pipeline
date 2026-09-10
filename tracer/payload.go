package tracer

import (
	"fmt"
	"time"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/core/types"
)

// PayloadCacheConfig controls the writer's experimental, in-memory execution cache.
// MaxBytes is a budget for estimated retained data, not a process RSS limit.
type PayloadCacheConfig struct {
	Enabled    bool   `json:"enabled"`
	MaxEntries int    `json:"max_entries"`
	MaxBytes   uint64 `json:"max_bytes"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func (c *PayloadCacheConfig) normalize() error {
	if c.MaxEntries < 0 || c.TTLSeconds < 0 || c.TTLSeconds > 3600 {
		return fmt.Errorf("invalid payload cache limits")
	}
	if c.MaxEntries == 0 {
		c.MaxEntries = 2
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = 256 * 1024 * 1024
	}
	if c.TTLSeconds == 0 {
		c.TTLSeconds = 30
	}
	return nil
}

func (t *PipelineTracer) PayloadCacheConfig() PayloadCacheConfig {
	return t.config.PayloadCache
}

// NewPayloadTracer starts an isolated collector. It does not initialize publishers,
// register a writer, commit state, or send notifications.
func NewPayloadTracer(header *types.Header) *PipelineTracer {
	t := &PipelineTracer{collectOnly: true}
	t.OnBlockStart(types.NewBlockWithHeader(header))
	return t
}

// SealPayload binds metadata that was not available while the block was executing.
// After sealing, the collector must not receive any more execution callbacks.
func (t *PipelineTracer) SealPayload(block *types.Block) error {
	if block == nil || !t.collectOnly || t.sealed || t.failed || t.block == nil || t.callTracer != nil {
		return fmt.Errorf("payload trace is not complete")
	}
	if t.block.BlockNumber != block.NumberU64() || t.block.BlockHeader.ParentHash != block.ParentHash() {
		return fmt.Errorf("payload trace parent or number mismatch")
	}
	if len(t.block.BlockFile.Txs) != len(block.Transactions()) {
		return fmt.Errorf("payload trace transaction count mismatch")
	}
	for i, tx := range block.Transactions() {
		collected := t.block.BlockFile.Txs[i]
		if collected.ID != tx.Hash().Hex() || collected.TransactionIndex != int64(i) {
			return fmt.Errorf("payload trace transaction mismatch at index %d", i)
		}
	}
	t.block.BlockHash = block.Hash()
	t.block.BlockHeader = util.BuildPilelineBlockHeader(block)
	t.block.BlockFile.Block = util.BuildPipelineBlock(block)
	t.sealed = true
	return nil
}

func (t *PipelineTracer) MatchesPayload(block *types.Block) bool {
	return t != nil && block != nil && t.collectOnly && t.sealed && !t.failed && t.block != nil &&
		t.callTracer == nil && !t.block.Committed && t.block.BlockHash == block.Hash() &&
		len(t.block.BlockFile.Txs) == len(block.Transactions())
}

// AdoptPayload moves a sealed collector into the import tracer. The source becomes
// unusable, so neither a retry nor another candidate can consume it a second time.
func (t *PipelineTracer) AdoptPayload(source *PipelineTracer, block *types.Block) error {
	if t.collectOnly || !source.MatchesPayload(block) {
		return fmt.Errorf("cannot adopt payload trace")
	}
	t.block, source.block = source.block, nil
	t.callTracer = nil
	t.failed = false
	t.sealed = false
	source.sealed = false
	// Keep the existing meaning of the import processing timestamp.
	t.block.BlockStartTime = time.Now()
	t.block.BlockFile.Block.ProcessStartTimestamp = t.block.BlockStartTime.UnixMilli()
	return nil
}

// EstimatedPayloadSize accounts for the immutable trace artifact and its variable
// length data. Shared publishers are deliberately not owned by the collector.
func (t *PipelineTracer) EstimatedPayloadSize() uint64 {
	if t.block == nil {
		return 0
	}
	b := t.block.BlockFile
	size := uint64(4096 + len(t.block.ChangeContracts)*128)
	for _, tx := range b.Txs {
		size += uint64(1024 + len(tx.ID) + len(tx.From) + len(tx.To) + len(tx.Input))
	}
	for _, traces := range [][]ptypes.Trace{b.Traces, b.ErrorTraces} {
		for _, trace := range traces {
			size += uint64(1024 + len(trace.ID) + len(trace.TxID) + len(trace.ParentTraceID) +
				len(trace.From) + len(trace.To) + len(trace.Input) + len(trace.Output) +
				len(trace.Error) + len(trace.TraceAddress)*8)
		}
	}
	for _, events := range [][]ptypes.Event{b.Events, b.ErrorEvents} {
		for _, event := range events {
			size += uint64(512 + len(event.ID) + len(event.Address) + len(event.Data))
			for _, topic := range event.Topics {
				size += uint64(32 + len(topic))
			}
		}
	}
	return size
}
