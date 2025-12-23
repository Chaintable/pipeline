package util

import (
	"math/big"
	"strings"
	"time"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/common/hexutil"
	"github.com/kaiachain/kaia/params"
)

func BuildPipelineBlock(rawBlock *types.Block) ptypes.Block {
	block := ptypes.Block{
		ID:                    rawBlock.Hash().Hex(),
		Height:                rawBlock.Number(),
		ParentID:              rawBlock.ParentHash().Hex(),
		BaseFeePerGas:         big.NewInt(0),
		Miner:                 strings.ToLower(rawBlock.Rewardbase().Hex()),
		GasLimit:              big.NewInt(int64(params.UpperGasLimit)),
		GasUsed:               big.NewInt(int64(rawBlock.GasUsed())),
		Timestamp:             rawBlock.Header().Time.Uint64(),
		ProcessStartTimestamp: time.Now().UnixMilli(),
	}
	return block
}

func BuildPilelineBlockHeader(block *types.Block) *ptypes.Header {
	blockHeader := ptypes.Header{
		Number:           (*hexutil.Big)(block.Number()),
		Hash:             block.Hash(),
		ParentHash:       block.ParentHash(),
		Nonce:            [8]byte{}, // always the empty nonce for klay
		MixHash:          common.BytesToHash(block.Header().MixHash),
		Sha3Uncles:       common.HexToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347"), // always the empty hash for klay
		LogsBloom:        block.Bloom(),
		StateRoot:        block.Root(),
		Miner:            block.Rewardbase(),
		Difficulty:       (*hexutil.Big)(block.Header().BlockScore),
		ExtraData:        hexutil.Bytes(block.Extra()),
		GasLimit:         hexutil.Uint64(params.UpperGasLimit),
		GasUsed:          hexutil.Uint64(block.GasUsed()),
		Timestamp:        hexutil.Uint64(block.Header().Time.Uint64()),
		TransactionsRoot: block.TxHash(),
		ReceiptsRoot:     block.ReceiptHash(),
	}
	return &blockHeader
}

func BuildPipelineTransaction(tx *types.Transaction, receipt *types.Receipt, from common.Address, baseFee *big.Int) ptypes.Transaction {
	to := receipt.ContractAddress
	if tx.To() != nil {
		to = *tx.To()
	}
	gasPrice := tx.GasPrice()
	transaction := ptypes.Transaction{
		ID:               tx.Hash().Hex(),
		From:             strings.ToLower(from.Hex()),
		To:               strings.ToLower(to.Hex()),
		Gas:              big.NewInt(int64(tx.Gas())),
		GasPrice:         gasPrice,
		GasUsed:          big.NewInt(int64(receipt.GasUsed)),
		Status:           receipt.Status == types.ReceiptStatusSuccessful,
		GasFeeCap:        common.Big0,
		GasTipCap:        common.Big0,
		Input:            tx.Data(),
		Nonce:            big.NewInt(int64(tx.Nonce())),
		TransactionIndex: int64(0), // TODO, 0 for now
		Value:            (*hexutil.Big)(tx.Value()),
	}
	return transaction
}
