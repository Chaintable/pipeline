package metrics

import (
	"github.com/rcrowley/go-metrics"
)

var (
	LatestBlockNumber = metrics.NewRegisteredGauge("pipeline/block_num", nil)

	LatestBlockTime = metrics.NewRegisteredGauge("pipeline/block_time", nil)

	LatestUploadedBlockNumber = metrics.NewRegisteredGauge("pipeline/latest_uploaded_block_number", nil)

	BlockProcessTimer = metrics.NewRegisteredTimer("pipeline/block_process", nil)

	BlockTxExecutionTimer = metrics.NewRegisteredTimer("pipeline/tx_execution", nil)

	BlockHeaderUploadTimer = metrics.NewRegisteredTimer("pipeline/block_header_upload", nil)

	StateDiffUploadTimer = metrics.NewRegisteredTimer("pipeline/state_diff_upload", nil)

	BlockFileUploadTimer = metrics.NewRegisteredTimer("pipeline/block_file_upload", nil)

	BlockFileValidationTimer = metrics.NewRegisteredTimer("pipeline/block_file_validation", nil)

	BlockPushTimer = metrics.NewRegisteredTimer("pipeline/block_push", nil)
)
