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
				Hash:        common.HexToHash("0x20fb5a41223aa538722b6766fb7b25c19b47168ae81ea738ad9ee176768367e4"),
				ParentHash:  common.HexToHash("0x814821e1c892bd6af9706e26e1457540b2d2dd668bbaaeaf83b5ca6ba698ccb0"),
				BlockNumber: 23093688,
				Timestamp:   1754642075,
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