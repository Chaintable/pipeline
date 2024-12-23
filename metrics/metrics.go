package metrics

import (
	"github.com/ethereum/go-ethereum/metrics"
)

var (
	LatestUploadedBlockNumber = metrics.NewRegisteredGauge("pipeline/latest_uploaded_block_number", nil)

	LatestPushedBlockNumber = metrics.NewRegisteredGauge("pipeline/latest_pushed_block_number", nil)

	BlockProcessTimer = metrics.NewRegisteredResettingTimer("pipeline/block_process", nil)

	BlockTxExecutionTimer = metrics.NewRegisteredResettingTimer("pipeline/tx_execution", nil)

	BlockUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/block_upload", nil)

	BlockHeaderUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/block_header_upload", nil)

	StateDiffUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/state_diff_upload", nil)

	BlockFileUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/block_file_upload", nil)

	BlockFileValidationTimer = metrics.NewRegisteredResettingTimer("pipeline/block_file_validation", nil)

	BlockPushTimer = metrics.NewRegisteredResettingTimer("pipeline/block_push", nil)
)
