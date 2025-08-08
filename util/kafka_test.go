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
				Hash:        common.HexToHash("0x22bb8dfd05df5b0617ea3d22eb072df91e4e425f30027b32d29d842196f2a824"),
				ParentHash:  common.HexToHash("0x1ceec5f210f06014ab8a1ff5e49366a79c6cbe83284ab6749c977b3bc65638c9"),
				BlockNumber: 2716678,
				Timestamp:   1480429483,
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