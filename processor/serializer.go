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

// s3Key: block/<blockHash>
func SerializeBlock(block *types.Block) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(block)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("block/%s", block.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: transaction/<blockHash>/<transactionHash>
func SerializeTransaction(transaction *types.Transaction) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(transaction)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("transaction/%s/%s", transaction.BlockHash.Hex(), transaction.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: receipt/<blockHash>/<transactionHash>
func SerializeReceipt(receipt *types.Receipt) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(receipt)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("receipt/%s/%s", receipt.BlockHash.Hex(), receipt.TransactionHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: trace/<blockHash>/<transactionHash>/<traceHash>
func SerializeTrace(trace types.CallFrame) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(trace)
	if err != nil {
		return nil, err
	}
	traceHash := trace.Hash()
	s3Key := fmt.Sprintf("trace/%s/%s/%s", trace.BlockHash.Hex(), trace.TransactionHash.Hex(), traceHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: event/<blockHash>/<transactionHash>/<eventHash>
func SerializeEvent(event *types.Event) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(event)
	if err != nil {
		return nil, err
	}
	eventHash := event.Hash()
	s3Key := fmt.Sprintf("event/%s/%s/%s", event.BlockHash.Hex(), event.TxHash.Hex(), eventHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: stateDiff/<blockHash>
func SerializeStateDiff(stateDiff *types.BlockStorageDiff) (*DataFile, error) {
	data, err := util.EncodeToRlp(stateDiff)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("stateDiff/%s", stateDiff.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}
