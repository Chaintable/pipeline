package tracer

import (
	"fmt"
	"github.com/Chaintable/pipeline/processor"
	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
	"sync"
	"time"
)

type ExtraInfo struct {
	BlockNumber uint64
	BlockHash   common.Hash
	BlockFile   *ptypes.BlockFile
	Tx          *types.Transaction
	From        common.Address
	BlockHeader *ptypes.Header
	BlockDiff   *ptypes.BlockStorageDiff
	BlockChange *ptypes.BlockChangeNotification
}

type Config struct {
	Region           string   `json:"region"`
	NodeXBucket      string   `json:"node_x_bucket"`
	ChainTableBucket string   `json:"chain_table_bucket"`
	Brokers          []string `json:"brokers"`
	Topic            string   `json:"topic"`
	ChainID          string   `json:"chain_id"`
}

var (
	NodeXPusher            *processor.PushProcessor
	ChainTableBucketPusher *processor.PushProcessor
	BlockCtx               *ExtraInfo
	BizChainID             string
)

func InitPipeline(region string, nodeXBucket string, chainTableBucket string, brokers []string, topic string, bizChainID string) (err error) {
	NodeXPusher, err = processor.NewPushProcessor(region, nodeXBucket, brokers, topic)
	if err != nil {
		return err
	}
	ChainTableBucketPusher, err = processor.NewPushProcessor(region, chainTableBucket, brokers, topic)
	if err != nil {
		return err
	}
	BizChainID = bizChainID
	return nil
}

func OnCommit() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var uploadErrs []error

	s3start := time.Now()

	// Helper function to handle errors safely
	handleError := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			uploadErrs = append(uploadErrs, err)
		}
	}

	// 上传 block head
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadBlockHeader(BlockCtx.BlockHeader)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 上传 state diff
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadBlockDiff(BlockCtx.BlockDiff)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 上传 block file
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadBlockFile(BlockCtx.BlockFile)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 上传 block file validation
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := uploadblockFileValidation(BlockCtx.BlockFile)
		if err != nil {
			handleError(err)
			return
		}
	}()

	// 等待所有上传完成
	wg.Wait()
	s3Elapsed := time.Since(s3start)

	// 检查是否有错误
	if len(uploadErrs) > 0 {
		for _, err := range uploadErrs {
			log.Error("Upload error", "err", err)
		}
		log.Crit("One or more uploads failed")
	}
	log.Info("Upload to s3", "elapsed", common.PrettyDuration(s3Elapsed))
}

func GenesisAllocToStateDiff(genesisAlloc types.GenesisAlloc) *ptypes.BlockStorageDiff {
	diff := &ptypes.BlockStorageDiff{}
	diff.NewAccounts = make([]ptypes.NewAccount, 0)
	diff.NewCodes = make([]ptypes.NewCode, 0)
	diff.StorageDiff = make([]ptypes.AccountStorageDiff, 0)
	diff.DeletedAccounts = make([]common.Hash, 0)
	for addr, acc := range genesisAlloc {
		diff.NewAccounts = append(diff.NewAccounts, ptypes.NewAccount{
			Address:  crypto.Keccak256Hash(addr[:]),
			Balance:  uint256.MustFromBig(acc.Balance),
			Nonce:    acc.Nonce,
			CodeHash: common.BytesToHash(acc.Code),
		})
		if len(acc.Code) > 0 {
			diff.NewCodes = append(diff.NewCodes, ptypes.NewCode{
				CodeHash: common.BytesToHash(acc.Code),
				Code:     acc.Code,
			})
		}
		values := make([]ptypes.IndexValuePair, 0)
		for index, v := range acc.Storage {
			value := uint256.NewInt(0)
			if len(v) > 0 {
				value = uint256.NewInt(0).SetBytes(v.Bytes())
			}
			values = append(values, ptypes.IndexValuePair{
				Index: index,
				Value: value,
			})
		}
		diff.StorageDiff = append(diff.StorageDiff, ptypes.AccountStorageDiff{
			Address: crypto.Keccak256Hash(addr[:]),
			Values:  values,
		})
	}
	return diff
}

func uploadBlockHeader(blockHeader *ptypes.Header) error {
	s3BlockFile, err := processor.SerializeHeader(BizChainID, blockHeader)
	if err != nil {
		return fmt.Errorf("failed to serialize block header: %v", err)
	}
	err = NodeXPusher.UploadFileToS3(s3BlockFile)
	if err != nil {
		return fmt.Errorf("failed to upload block header: %v", err)
	}
	return nil
}

func uploadBlockDiff(blockDiff *ptypes.BlockStorageDiff) error {
	s3file, err := processor.SerializeStateDiff(BizChainID, blockDiff)
	if err != nil {
		return fmt.Errorf("failed to serialize state diff: %v", err)
	}
	err = NodeXPusher.UploadFileToS3(s3file)
	if err != nil {
		return fmt.Errorf("failed to upload state diff: %v", err)
	}
	return nil
}

func uploadBlockFile(blockFile *ptypes.BlockFile) error {
	s3file, err := processor.SerializeFile(BizChainID, blockFile)
	if err != nil {
		return fmt.Errorf("failed to serialize block file: %v", err)
	}
	err = ChainTableBucketPusher.UploadFileToS3(s3file)
	if err != nil {
		return fmt.Errorf("failed to upload block file: %v", err)
	}
	return nil
}

func uploadblockFileValidation(blockFile *ptypes.BlockFile) error {
	blockFileValidation, err := processor.SerializeFileValidation(BizChainID, blockFile)
	if err != nil {
		return fmt.Errorf("failed to serialize block file validation: %v", err)
	}
	err = ChainTableBucketPusher.UploadFileToS3(blockFileValidation)
	if err != nil {
		return fmt.Errorf("failed to upload block file validation: %v", err)
	}
	return nil
}
