package processor

import (
	"fmt"

	"github.com/DeBankDeFi/pipeline/types"
	"github.com/DeBankDeFi/pipeline/util"
)

type DataFile struct {
	S3key string
	Data  []byte
}

// s3Key: <blockHash>/block
func SerializeBlock(block *types.Block) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(block)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/block", block.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <blockHash>/transaction/<transactionHash>
func SerializeTransaction(transaction *types.Transaction) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(transaction)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/transaction/%s", transaction.BlockHash.Hex(), transaction.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <blockHash>/receipt/<transactionHash>
func SerializeReceipt(receipt *types.Receipt) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(receipt)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/receipt/%s", receipt.BlockHash.Hex(), receipt.TransactionHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <blockHash>/trace/<transactionHash>/<traceHash>
func SerializeTrace(trace types.CallFrame) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(trace)
	if err != nil {
		return nil, err
	}
	traceHash := trace.Hash()
	s3Key := fmt.Sprintf("%s/trace/%s/%s", trace.BlockHash.Hex(), trace.TransactionHash.Hex(), traceHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <blockHash>/event/<transactionHash>/<eventHash>
func SerializeEvent(event *types.Event) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(event)
	if err != nil {
		return nil, err
	}
	eventHash := event.Hash()
	s3Key := fmt.Sprintf("%s/event/%s/%s", event.BlockHash.Hex(), event.TxHash.Hex(), eventHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <blockHash>/stateDiff
func SerializeStateDiff(stateDiff *types.BlockStorageDiff) (*DataFile, error) {
	data, err := util.EncodeToRlp(stateDiff)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/stateDiff", stateDiff.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}
