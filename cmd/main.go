package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"

	"github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	s3client, err := util.NewS3Client("ap-northeast-1")
	if err != nil {
		panic(err)
	}

	for i := 0; i < 10; i++ {
		err = ValidateBlockFileFromS3(s3client, "chaintable-pipeline-test", int64(i))
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Block file is valid")
}

func ValidateBlockFileFromS3(downloader *s3.Client, bucket string, blockNumber int64) error {
	blockValidation, id, err := DownloadAndDecodeMetaFromS3(downloader, bucket, 1, blockNumber)
	if err != nil {
		return err
	}

	blockFile := types.BlockFile{}

	key := fmt.Sprintf("a/%s", id)

	err = downloadFileFromS3(downloader, bucket, key, &blockFile)
	if err != nil {
		return fmt.Errorf("failed to download and decode block file: %w", err)
	}

	if blockFile.Validation().ValidationHash != blockValidation.ValidationHash {
		return fmt.Errorf("validation hash mismatch")
	}

	return nil
}

func DownloadAndDecodeMetaFromS3(downloader *s3.Client, bucket string, chainID int64, blockNumber int64) (*types.BlockValidation, string, error) {
	prefix := fmt.Sprintf("%d/%d/", chainID, blockNumber)
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
		err := downloadFileFromS3(downloader, bucket, key, &blockValidation)
		if err != nil {
			return nil, "", fmt.Errorf("failed to download and decode JSON: %w", err)
		}

		if !blockValidation.IsFork {
			return &blockValidation, path.Base(key), nil
		}
	}

	return nil, "", fmt.Errorf("no valid block found")
}

func downloadFileFromS3(downloader *s3.Client, bucket string, key string, target any) error {
	input := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}

	result, err := downloader.GetObject(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to download file from S3: %w", err)
	}
	defer result.Body.Close()

	return decodeGzipJson(result.Body, target)
}

func decodeGzipJson(readerCloser io.Reader, target any) error {
	gz, err := gzip.NewReader(readerCloser)
	if err != nil {
		return err
	}
	return json.NewDecoder(gz).Decode(target)
}
