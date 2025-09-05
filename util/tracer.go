package util

import (
	"math/big"
	"strings"
	"time"

	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func BuildPipelineBlock(rawBlock *ptypes.EvmHeader) ptypes.Block {
	block := ptypes.Block{
		ID:                    rawBlock.Hash.Hex(),
		Height:                rawBlock.Number,
		ParentID:              rawBlock.ParentHash.Hex(),
		BaseFeePerGas:         big.NewInt(0),
		Miner:                 strings.ToLower(rawBlock.Coinbase.Hex()),
		GasLimit:              big.NewInt(int64(rawBlock.GasLimit)),
		GasUsed:               big.NewInt(int64(rawBlock.GasUsed)),
		Timestamp:             rawBlock.Time,
		ProcessStartTimestamp: time.Now().UnixMilli(),
	}
	if rawBlock.BaseFee != nil {
		block.BaseFeePerGas = rawBlock.BaseFee
	}
	return block
}

func BuildPilelineBlockHeader(block *ptypes.EvmHeader) *ptypes.Header {
	blockHeader := ptypes.Header{
		Number:           (*hexutil.Big)(block.Number),
		Hash:             block.Hash,
		ParentHash:       block.ParentHash,
		Nonce:            types.BlockNonce{},
		MixHash:          common.Hash{},
		Sha3Uncles:       types.EmptyUncleHash,
		StateRoot:        block.Root,
		Miner:            block.Coinbase,
		Difficulty:       (*hexutil.Big)(common.Big0),
		GasLimit:         hexutil.Uint64(block.GasLimit),
		GasUsed:          hexutil.Uint64(block.GasUsed),
		Timestamp:        hexutil.Uint64(block.Time),
		TransactionsRoot: block.TxHash,
	}
	if block.BaseFee != nil {
		blockHeader.BaseFeePerGas = (*hexutil.Big)(block.BaseFee)
	}
	return &blockHeader
}

func BuildPipelineTransaction(tx *types.Transaction, receipt *types.Receipt, from common.Address, baseFee *big.Int) ptypes.Transaction {
	to := receipt.ContractAddress
	if tx.To() != nil {
		to = *tx.To()
	}
	transaction := ptypes.Transaction{
		ID:               tx.Hash().Hex(),
		From:             strings.ToLower(from.Hex()),
		To:               strings.ToLower(to.Hex()),
		Gas:              big.NewInt(int64(tx.Gas())),
		GasPrice:         tx.GasPrice(),
		GasUsed:          big.NewInt(int64(receipt.GasUsed)),
		Status:           receipt.Status == types.ReceiptStatusSuccessful,
		GasFeeCap:        common.Big0,
		GasTipCap:        common.Big0,
		Input:            tx.Data(),
		Nonce:            big.NewInt(int64(tx.Nonce())),
		TransactionIndex: int64(receipt.TransactionIndex),
		Value:            (*hexutil.Big)(tx.Value()),
	}
	switch tx.Type() {
	case types.DynamicFeeTxType:
		transaction.GasFeeCap = tx.GasFeeCap()
		transaction.GasTipCap = tx.GasTipCap()
	}
	return transaction
}
