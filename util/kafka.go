package util

import (
	"bytes"
	"context"
	"fmt"
	"time"

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

// 获取最后一个BlockChangeNotification
func GetLastBlockNotice(reader *kafka.Reader) (*types.BlockChangeNotification, error) {
	err := reader.SetOffset(kafka.LastOffset)
	if err != nil {
		return nil, err
	}

	// 尝试读取一条消息，如果读不到则说明没有消息
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		if err == context.DeadlineExceeded {
			// 如果超时且没有读取到消息，则认为 Topic 为空
			return nil, nil
		}
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
