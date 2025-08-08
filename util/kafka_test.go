package util

import (
	"context"
	"testing"

	"github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
)

func TestKafkaReader(t *testing.T) {
	reader := NewKafkaReader([]string{"b-1.chaintablenodexpi.pz2h9z.c4.kafka.ap-northeast-1.amazonaws.com:9092"}, "nodex_pipeline_test", "")
	reader.SetOffset(0)
	lag, a := reader.ReadLag(context.Background())
	if a != nil {
		t.Error(a)
	}
	t.Log(lag)

	msg, err := GetLastBlockNotice(reader)
	if err != nil {
		t.Error(err)
	}
	t.Log(msg)
}

func TestKafkaWriter(t *testing.T) {
	writer := NewKafkaWriter([]string{"localhost:9092"}, "nodex_pipeline_1_tmp")
	if writer == nil {
		t.Error("Failed to create Kafka writer")
	}
	err := WriteBlockNotice(writer, &types.BlockChangeNotification{
		ChangeType: 1,
		NewBlocks: []types.BlockContext{
			{
				Hash:        common.HexToHash("0xcfb992a205919e03da9d2235203316bfa2aaa6e1cf7d5bb95bb0b5d2a2fe3a66"),
				ParentHash:  common.HexToHash("0xd6a0a4f986289f3d2e77f9e19779c7c8e1341d612d10ad4c59e54ceffdaebb61"),
				BlockNumber: 2713851,
				Timestamp:   1480374012,
			},
		},
		DropBlocks: nil,
	})
	if err != nil {
		t.Error(err)
	}
	t.Log("write block notice success")
}

// 查看kafka的offset
func TestKafkaReader1(t *testing.T) {
	reader := NewKafkaReader([]string{"localhost:9092"}, "nodex_pipeline_1_tmp", "")
	reader.SetOffset(0)
	lag, a := reader.ReadLag(context.Background())
	if a != nil {
		t.Error(a)
	}
	t.Logf("lag: %d", lag)

	// 输出最后一条消息
	msg, err := GetLastBlockNotice(reader)
	if err != nil {
		t.Error(err)
	}
	t.Logf("last message: %+v", msg)
}