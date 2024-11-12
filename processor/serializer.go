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

// s3key: chain_id/block_height/block_id
// 外部s3
func SerializeFile(chainID *hexutil.Big, blockFile *types.BlockFile) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(blockFile)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/%d/%s", chainID.String(), blockFile.Block.Height, blockFile.Block.ID.String())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/header
// 内部s3
func SerializeHeader(chainID *hexutil.Big, header *types.HeaderWithValidationHash) (*DataFile, error) {
	data, err := util.EncodeToJsonGzip(header)
	if err != nil {
		return nil, err
	}
	s3Key := fmt.Sprintf("%s/%s/block", chainID.String(), header.Header.Hash.String())
	return &DataFile{
		S3key: s3Key,
		Data:  data,
	}, nil
}

// s3Key: <chainID>/<blockHash>/stateDiff
// 内部s3
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
