package util

import (
	"bytes"
	"context"
	"fmt"

	"github.com/DeBankDeFi/pipeline/types"
	"github.com/segmentio/kafka-go"
)

func NewKafkaReader(brokers []string, topic string, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
}

func IsTopicEmpty(broker, topic string) (bool, error) {
	// 创建 Kafka 客户端的连接
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return false, fmt.Errorf("failed to connect to Kafka: %v", err)
	}
	defer conn.Close()

	// 获取该 Topic 的分区信息
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return false, fmt.Errorf("failed to read partitions: %v", err)
	}

	// 遍历所有分区，检查每个分区的最新偏移量
	for _, p := range partitions {
		partitionConn, err := kafka.DialPartition(context.Background(), "tcp", broker, kafka.Partition{
			Topic: topic,
			ID:    p.ID,
		})
		if err != nil {
			return false, fmt.Errorf("failed to connect to partition %d: %v", p.ID, err)
		}
		defer partitionConn.Close()

		first, latest, err := partitionConn.ReadOffsets()
		if err != nil {
			return false, fmt.Errorf("failed to get offset: %v", err)
		}

		if first != latest {
			return false, nil
		}
	}

	return true, nil
}

// 获取最后一个BlockChangeNotification
func GetLastBlockNotice(reader *kafka.Reader) (*types.BlockChangeNotification, error) {
	err := reader.SetOffset(kafka.LastOffset)
	if err != nil {
		return nil, err
	}

	msg, err := reader.ReadMessage(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error reading message: %v", err)
	}

	if !bytes.Equal(msg.Key, []byte("NewBlock")) {
		return nil, fmt.Errorf("last message is not NewBlock")
	}

	blockNotice := &types.BlockChangeNotification{}
	err = DecodeFromGzipJson(msg.Value, blockNotice)
	if err != nil {
		return nil, err
	}

	return blockNotice, nil
}

func NewKafkaWriterForBlockNotice(brokers []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
	}
}

func WriteBlockNotice(writer *kafka.Writer, blockNotice *types.BlockChangeNotification) error {
	value, err := EncodeToJsonGzip(blockNotice)
	if err != nil {
		return err
	}
	err = writer.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte("NewBlock"),
		Value: value,
	})
	if err != nil {
		return err
	}
	return nil
}
