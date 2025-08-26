package metrics

import (
	"github.com/ava-labs/libevm/metrics"
)

var (
	LatestBlockNumber = metrics.NewRegisteredGauge("pipeline/block_num", nil)

	LatestBlockTime = metrics.NewRegisteredGauge("pipeline/block_time", nil)

	LatestUploadedBlockNumber = metrics.NewRegisteredGauge("pipeline/latest_uploaded_block_number", nil)

	NodeInfo = metrics.NewRegisteredGaugeInfo("pipeline/node_info", nil)

	BlockProcessTimer = metrics.GetOrRegisterCounter("pipeline/block_process", nil)

	BlockTxExecutionTimer = metrics.GetOrRegisterCounter("pipeline/tx_execution", nil)

	BlockHeaderUploadTimer = metrics.GetOrRegisterCounter("pipeline/block_header_upload", nil)

	StateDiffUploadTimer = metrics.GetOrRegisterCounter("pipeline/state_diff_upload", nil)

	BlockFileUploadTimer = metrics.GetOrRegisterCounter("pipeline/block_file_upload", nil)

	BlockFileValidationTimer = metrics.GetOrRegisterCounter("pipeline/block_file_validation", nil)

	BlockPushTimer = metrics.GetOrRegisterCounter("pipeline/block_push", nil)
)
