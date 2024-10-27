package processor

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/DeBankDeFi/pipeline/types"
	"github.com/DeBankDeFi/pipeline/util"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/segmentio/kafka-go"
)

// PushProcessor is a processor that pushes data to s3 and kafka
type PushProcessor struct {
	Bucket          string
	Uploader        *s3manager.Uploader
	KafkaReader     *kafka.Reader
	KafkaWriter     *kafka.Writer
	LastBlockNotice *types.BlockChangeNotification
}

func NewPushProcessor(region string, bucket string, brokers []string, topic string) (*PushProcessor, error) {
	kafkaReader := util.NewKafkaReader(brokers, topic, "push-processor")
	kafkaWriter := util.NewKafkaWriterForBlockNotice(brokers, topic)
	s3Uploader, err := util.NewS3Uploader(region)
	if err != nil {
		return nil, err
	}

	var lastBlockNotice *types.BlockChangeNotification

	empty, err := util.IsTopicEmpty(brokers[0], topic)
	if err != nil {
		return nil, err
	}

	if !empty {
		lastBlockNotice, err = util.GetLastBlockNotice(kafkaReader)
		if err != nil {
			return nil, err
		}
	}

	return &PushProcessor{
		Bucket:          bucket,
		Uploader:        s3Uploader,
		KafkaReader:     kafkaReader,
		KafkaWriter:     kafkaWriter,
		LastBlockNotice: lastBlockNotice,
	}, nil
}

func (p *PushProcessor) UploadFileToS3(file *DataFile) error {
	times := 0
	for {
		err := util.UploadFileToS3(p.Uploader, p.Bucket, file.S3key, file.Data)
		if err != nil {
			if times > 3 {
				return err
			}
			time.Sleep(time.Second)
			times++
			continue
		}
		break
	}
	return nil
}

func (p *PushProcessor) UploadFilesToS3(files []*DataFile) error {
	var wg sync.WaitGroup
	var errs []error
	var lock sync.Mutex
	for _, file := range files {
		wg.Add(1)
		go func(file *DataFile) {
			times := 0
			for {
				err := util.UploadFileToS3(p.Uploader, p.Bucket, file.S3key, file.Data)
				if err != nil {
					if times > 3 {
						lock.Lock()
						errs = append(errs, err)
						lock.Unlock()
						log.Printf("failed to upload file to s3: %s", err)
						wg.Done()
						return
					}
					time.Sleep(time.Second)
					times++
					continue
				}
				break
			}
			wg.Done()
		}(file)
	}
	wg.Wait()
	if len(errs) > 0 {
		return fmt.Errorf("failed to upload files to s3: %v", errs)
	}
	return nil
}

func (p *PushProcessor) LastPushedBlock() *types.BlockContext {
	if p.LastBlockNotice == nil {
		return nil
	}
	return &p.LastBlockNotice.NewBlocks[len(p.LastBlockNotice.NewBlocks)-1]
}

func (p *PushProcessor) PushBlockChangeNotification(blockNotice *types.BlockChangeNotification) error {
	if p.LastPushedBlock() == nil && blockNotice.NewBlocks[0].BlockNumber != 0 {
		return fmt.Errorf("last pushed block is empty but new block number is not 0")
	}

	if p.LastPushedBlock() != nil &&
		(p.LastPushedBlock().BlockNumber >= blockNotice.NewBlocks[len(blockNotice.NewBlocks)-1].BlockNumber) {
		return nil
	}

	if p.LastPushedBlock() != nil {
		if p.LastBlockNotice.ChangeType == 1 {
			if p.LastPushedBlock().Hash != blockNotice.NewBlocks[0].ParrentHash {
				return fmt.Errorf("last pushed block hash is not equal to new block parent hash")
			}
		}
		if p.LastBlockNotice.ChangeType == 2 {
			if p.LastPushedBlock().Hash != blockNotice.DropBlocks[len(blockNotice.DropBlocks)-1].Hash {
				return fmt.Errorf("last pushed block hash is not equal to drop block hash")
			}
		}
	}

	// 将区块变更通知写入Kafka
	err := util.WriteBlockNotice(p.KafkaWriter, blockNotice)
	if err != nil {
		return fmt.Errorf("写入区块变更通知到Kafka失败: %v", err)
	}

	// 更新最新的区块通知
	p.LastBlockNotice = blockNotice
	return nil
}

func (p *PushProcessor) Close() {
	p.KafkaReader.Close()
	p.KafkaWriter.Close()
}
