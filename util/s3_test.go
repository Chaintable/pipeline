package util

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestS3(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}
	res, err := s3client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		t.Errorf("Expected error, got nil")
	}
	if res == nil {
		t.Errorf("Expected response, got nil")
	}
	t.Logf("Tested NewS3Client() successfully, %+v", res)
}
