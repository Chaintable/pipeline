package metrics

import (
	"fmt"

	"github.com/ava-labs/libevm/metrics"
)

var (
	LatestBlockNumber = metrics.NewRegisteredGauge("pipeline/block_num", nil)

	LatestBlockTime = metrics.NewRegisteredGauge("pipeline/block_time", nil)

	LatestUploadedBlockNumber = metrics.NewRegisteredGauge("pipeline/latest_uploaded_block_number", nil)

	NodeInfo = metrics.NewRegisteredGaugeInfo("pipeline/node_info", nil)

	BlockProcessTimer = metrics.GetOrRegisterTimer("pipeline/block_process", nil)

	BlockTxExecutionTimer = metrics.GetOrRegisterTimer("pipeline/tx_execution", nil)

	BlockHeaderUploadTimer = metrics.GetOrRegisterTimer("pipeline/block_header_upload", nil)

	StateDiffUploadTimer = metrics.GetOrRegisterTimer("pipeline/state_diff_upload", nil)

	BlockFileUploadTimer = metrics.GetOrRegisterTimer("pipeline/block_file_upload", nil)

	BlockFileValidationTimer = metrics.GetOrRegisterTimer("pipeline/block_file_validation", nil)

	BlockPushTimer = metrics.NewRegisteredResettingTimer("pipeline/block_push", nil)

	// S3UploadRetryCounter 累计 S3 上传重试次数，限流抬头即可告警。
	S3UploadRetryCounter = metrics.NewRegisteredCounter("pipeline/s3_upload_retry", nil)
)

// KafkaWriteTimer returns the write timer for a Kafka topic. The underlying
// metrics registry does not support labels, so the topic is part of the metric
// name (for example, pipeline/kafka_write/orders).
func KafkaWriteTimer(topic string) metrics.ResettingTimer {
	return metrics.GetOrRegisterResettingTimer(fmt.Sprintf("pipeline/kafka_write/%s", topic), nil)
}
