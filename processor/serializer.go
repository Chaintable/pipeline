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

// s3Key: transaction/<transactionHash>
func SerializeTransaction(transaction types.Transaction) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(transaction)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("transaction/%s", transaction.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: receipt/<transactionHash>
func SerializeReceipt(receipt types.Receipt) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(receipt)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("receipt/%s", receipt.TransactionHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: trace/<transactionHash>/<traceHash>
func SerializeTrace(trace types.CallFrame) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(trace)
	if err != nil {
		return nil, err
	}
	traceHash := trace.Hash()
	s3Key := fmt.Sprintf("trace/%s/%s", trace.TransactionHash.Hex(), traceHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: event/<transactionHash>/<eventHash>
func SerializeEvent(event types.Event) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(event)
	if err != nil {
		return nil, err
	}
	eventHash := event.Hash()
	s3Key := fmt.Sprintf("event/%s/%s", event.TxHash.Hex(), eventHash.Hex())
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

// todo 并行化
func SerializeAll(block *types.Block, transactions []types.Transaction, receipts []types.Receipt, traces []types.CallFrame, events []types.Event, stateDiff *types.BlockStorageDiff) ([]*DataFile, error) {
	files := make([]*DataFile, 0)
	blockFile, err := SerializeBlock(block)
	if err != nil {
		return nil, err
	}
	files = append(files, blockFile)
	for _, tx := range transactions {
		txFile, err := SerializeTransaction(tx)
		if err != nil {
			return nil, err
		}
		files = append(files, txFile)
	}
	for _, receipt := range receipts {
		receiptFile, err := SerializeReceipt(receipt)
		if err != nil {
			return nil, err
		}
		files = append(files, receiptFile)
	}
	for _, trace := range traces {
		traceFile, err := SerializeTrace(trace)
		if err != nil {
			return nil, err
		}
		files = append(files, traceFile)
	}
	for _, event := range events {
		eventFile, err := SerializeEvent(event)
		if err != nil {
			return nil, err
		}
		files = append(files, eventFile)
	}
	stateDiffFile, err := SerializeStateDiff(stateDiff)
	if err != nil {
		return nil, err
	}
	files = append(files, stateDiffFile)
	return files, nil
}
