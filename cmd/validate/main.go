package main

import (
	"context"
	"flag"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

func main() {
	concurrency := flag.Int("concurrency", 100, "Number of goroutines to use for validating blocks")
	startNum := flag.Int("start", 1, "Start block number")
	endNum := flag.Int("end", 20000000, "End block number (非包含)")
	region := flag.String("region", "ap-northeast-1", "AWS region for S3")
	bucket0 := flag.String("innerBucket", "chaintable-nodex-pipeline-test", "S3 bucket name")
	bucket1 := flag.String("outerBucket", "chaintable-pipeline-test", "S3 bucket nam")
	chainID0 := flag.String("chainIDOld", "1", "Chain ID")
	chainID1 := flag.String("chainIDNew", "a", "Chain ID")

	s3client, err := util.NewS3Client(*region)
	if err != nil {
		panic(err)
	}

	workch := make(chan int, *concurrency)
	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				blockNumber, ok := <-workch
				if !ok {
					return
				}

				err := ValidateFromS3(s3client, *bucket0, *bucket1, *chainID0, *chainID1, int64(blockNumber))
				if err != nil {
					panic(fmt.Errorf("blocknum %d, err %v", blockNumber, err))
				}
			}
		}()
	}

	start := time.Now()
	for i := *startNum; i < *endNum; i++ {
		if i%1000 == 0 {
			el := time.Now().Unix() - start.Unix()
			rate := 1.0
			rate = float64(i-*startNum) / float64(el)
			fmt.Printf("start %d, rate %f \n", i, rate)
		}
		workch <- i
	}
	close(workch)
	wg.Wait()

	// err = ValidateFromS3(s3client, "chaintable-nodex-pipeline-test", "chaintable-pipeline-test", 1814633)
	// if err != nil {
	// 	panic(err)
	// }
	fmt.Println("Block file is valid")
}

func ValidateFromS3(downloader *s3.Client, innerBucket, outerBucket, chainID0, chainID1 string, blockNumber int64) error {
	blockValidation, id, err := DownloadAndDecodeMetaFromS3(downloader, outerBucket, chainID0, blockNumber)
	if err != nil {
		return err
	}

	blockFile := types.BlockFile{}

	key := fmt.Sprintf("%s/%s", chainID1, id)

	err = util.DownloadFileFromS3Json(downloader, outerBucket, key, &blockFile)
	if err != nil {
		return fmt.Errorf("failed to download and decode block file: %w", err)
	}

	if blockFile.Validation().ValidationHash != blockValidation.ValidationHash {
		return fmt.Errorf("validation hash mismatch")
	}

	err = ValidateStateFileFromS3(downloader, innerBucket, id)
	if err != nil {
		return fmt.Errorf("failed to validate state file: %w", err)
	}

	return nil
}

func ValidateStateFileFromS3(downloader *s3.Client, innerBucket string, id string) error {
	rawBlockKey := fmt.Sprintf("a/%s/block", id)
	header := types.Header{}
	err := util.DownloadFileFromS3Json(downloader, innerBucket, rawBlockKey, &header)
	if err != nil {
		return fmt.Errorf("failed to download and decode header: %w", err)
	}
	root := header.StateRoot

	key0 := fmt.Sprintf("1/%s/stateDiff", root)
	stateDiff0 := types.BlockStorageDiff{}
	err = util.DownloadFileFromS3Rlp(downloader, innerBucket, key0, &stateDiff0)
	if err != nil {
		return fmt.Errorf("failed to download and decode state diff: %w", err)
	}
	key1 := fmt.Sprintf("a/%s/stateDiff", root)
	stateDiff1 := types.BlockStorageDiff{}
	err = util.DownloadFileFromS3Rlp(downloader, innerBucket, key1, &stateDiff1)
	if err != nil {
		return fmt.Errorf("failed to download and decode state diff: %w", err)
	}

	newAccounts := make(map[common.Hash]types.NewAccount)
	// geth会重复添加system contract account
	for _, account := range stateDiff0.NewAccounts {
		newAccounts[account.Address] = account
	}
	for _, account := range stateDiff1.NewAccounts {
		if _, ok := newAccounts[account.Address]; !ok {
			continue
		}
		if newAccounts[account.Address].Balance.Cmp(account.Balance) != 0 {
			return fmt.Errorf("balance mismatch")
		}
		if newAccounts[account.Address].Nonce != account.Nonce {
			return fmt.Errorf("nonce mismatch")
		}
		if newAccounts[account.Address].CodeHash != account.CodeHash {
			return fmt.Errorf("code hash mismatch")
		}
	}

	delAccouts := make(map[common.Hash]struct{})
	// erigon会重复删除system account
	for _, account := range stateDiff1.DeletedAccounts {
		delAccouts[account] = struct{}{}
	}
	for _, account := range stateDiff0.DeletedAccounts {
		if _, ok := delAccouts[account]; !ok {
			return fmt.Errorf("deleted account not found")
		}
	}

	storageDiff := make(map[common.Hash]types.AccountStorageDiff)

	for _, diff := range stateDiff1.StorageDiff {
		storageDiff[diff.Address] = diff
	}

	for _, diff := range stateDiff0.StorageDiff {
		if _, ok := storageDiff[diff.Address]; !ok {
			return fmt.Errorf("storage diff not found")
		}
		m := make(map[common.Hash]*uint256.Int)
		for _, val := range diff.Values {
			m[val.Index] = val.Value
		}
		for _, val := range storageDiff[diff.Address].Values {
			if _, ok := m[val.Index]; !ok {
				continue
			}
			if !m[val.Index].Eq(val.Value) {
				return fmt.Errorf("storage not eq")
			}
		}
	}

	codeDiff := make(map[common.Hash]types.NewCode)
	for _, code := range stateDiff1.NewCodes {
		if code.CodeHash != crypto.Keccak256Hash(nil) {
			codeDiff[code.CodeHash] = code
		}
	}
	for _, code := range stateDiff0.NewCodes {
		if code.CodeHash != crypto.Keccak256Hash(nil) {
			if _, ok := codeDiff[code.CodeHash]; !ok {
				return fmt.Errorf("new code not found")
			}
		}
	}
	return nil
}

func DownloadAndDecodeMetaFromS3(downloader *s3.Client, bucket string, chainID0 string, blockNumber int64) (*types.BlockValidation, string, error) {
	prefix := fmt.Sprintf("%s/%d/", chainID0, blockNumber)
	input := &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	}

	output, err := downloader.ListObjectsV2(context.TODO(), input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list objects from S3: %w", err)
	}

	for _, object := range output.Contents {
		key := *object.Key
		var blockValidation types.BlockValidation
		err := util.DownloadFileFromS3Json(downloader, bucket, key, &blockValidation)
		if err != nil {
			return nil, "", fmt.Errorf("failed to download and decode JSON: %w", err)
		}

		if !blockValidation.IsFork {
			return &blockValidation, path.Base(key), nil
		}
	}

	return nil, "", fmt.Errorf("no valid block found")
}
