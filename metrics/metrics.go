package metrics

import (
	"github.com/ethereum/go-ethereum/metrics"
)

var (
	LatestBlockNumber = metrics.NewRegisteredGauge("pipeline/block_num", nil)

	LatestBlockTime = metrics.NewRegisteredGauge("pipeline/block_time", nil)

	LatestUploadedBlockNumber = metrics.NewRegisteredGauge("pipeline/latest_uploaded_block_number", nil)

	NodeInfo = metrics.NewRegisteredGaugeInfo("pipeline/node_info", nil)

	BlockProcessTimer = metrics.NewRegisteredResettingTimer("pipeline/block_process", nil)

	BlockTxExecutionTimer = metrics.NewRegisteredResettingTimer("pipeline/tx_execution", nil)

	BlockHeaderUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/block_header_upload", nil)

	StateDiffUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/state_diff_upload", nil)

	BlockFileUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/block_file_upload", nil)

	BlockFileValidationTimer = metrics.NewRegisteredResettingTimer("pipeline/block_file_validation", nil)

	BlockStateLoadTimer = metrics.NewRegisteredResettingTimer("pipeline/state_load_upload", nil)

	BlockRawUploadTimer = metrics.NewRegisteredResettingTimer("pipeline/block_raw_upload", nil)

	BlockPushTimer = metrics.NewRegisteredResettingTimer("pipeline/block_push", nil)
)
