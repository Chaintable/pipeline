package processor

import (
	"fmt"
	"github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type DataFile struct {
	S3key string
	Data  []byte
}

// s3Key: <chainID>/<blockHash>/block
func SerializeBlock(chainID *hexutil.Big, block *types.Block) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(block)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/%s/block", chainID.String(), block.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/transaction/<transactionHash>
func SerializeTransaction(chainID *hexutil.Big, transaction *types.Transaction) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(transaction)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/%s/transaction/%s", chainID.String(), transaction.BlockHash.Hex(), transaction.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/receipt/<transactionHash>
func SerializeReceipt(chainID *hexutil.Big, receipt *types.Receipt) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(receipt)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/%s/receipt/%s", chainID.String(), receipt.BlockHash.Hex(), receipt.TransactionHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/trace/<transactionHash>/<traceHash>
func SerializeTrace(chainID *hexutil.Big, trace types.CallFrame) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(trace)
	if err != nil {
		return nil, err
	}
	traceHash := trace.Hash()
	s3Key := fmt.Sprintf("%s/%s/trace/%s/%s", chainID.String(), trace.BlockHash.Hex(), trace.TransactionHash.Hex(), traceHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/event/<transactionHash>/<eventHash>
func SerializeEvent(chainID *hexutil.Big, event *types.Event) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(event)
	if err != nil {
		return nil, err
	}
	eventHash := event.Hash()
	s3Key := fmt.Sprintf("%s/%s/event/%s/%s", chainID.String(), event.BlockHash.Hex(), event.TxHash.Hex(), eventHash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/stateDiff
func SerializeStateDiff(chainID *hexutil.Big, stateDiff *types.BlockStorageDiff) (*DataFile, error) {
	data, err := util.EncodeToRlp(stateDiff)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/%s/stateDiff", chainID.String(), stateDiff.Hash.Hex())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}
