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

func BuildPipelineBlock(rawBlock *types.Block) ptypes.Block {
	// Use MixDigest as the canonical block identifier on iotex: the upstream
	// iotex fork stores the iotex-native block hash there when synthesizing a
	// geth-shaped block (see Chaintable/iotex-core-x blockchain.ConvertToGethBlock),
	// and downstream consumers (leafage / canonical events) bind on the iotex
	// hash, not on the synthetic geth Header.Hash() that depends on every
	// auxiliary header field. The v0.0.64-iotex-v2.3.8-debank-1 release carried
	// this; the v0.0.65 cut was branched from pipeline main and inadvertently
	// reverted to rawBlock.Hash(). docs/v2.4.1-plan/6-2-test-report.md §3.4.1.
	block := ptypes.Block{
		ID:                    rawBlock.MixDigest().Hex(),
		Height:                rawBlock.Number(),
		ParentID:              rawBlock.ParentHash().Hex(),
		BaseFeePerGas:         big.NewInt(0),
		Miner:                 strings.ToLower(rawBlock.Coinbase().Hex()),
		GasLimit:              big.NewInt(int64(rawBlock.GasLimit())),
		GasUsed:               big.NewInt(int64(rawBlock.GasUsed())),
		Timestamp:             rawBlock.Time(),
		ProcessStartTimestamp: time.Now().UnixMilli(),
	}
	if rawBlock.Header().BaseFee != nil {
		block.BaseFeePerGas = rawBlock.Header().BaseFee
	}
	return block
}

func BuildPilelineBlockHeader(block *types.Block) *ptypes.Header {
	// Hash mirrors BuildPipelineBlock's ID choice: use the iotex-native hash
	// stored in MixDigest by the fork's ConvertToGethBlock, not the synthetic
	// geth header hash. See BuildPipelineBlock above for the rationale.
	blockHeader := ptypes.Header{
		Number:           (*hexutil.Big)(block.Number()),
		Hash:             block.MixDigest(),
		ParentHash:       block.ParentHash(),
		Nonce:            block.Header().Nonce,
		MixHash:          block.MixDigest(),
		Sha3Uncles:       block.UncleHash(),
		LogsBloom:        block.Bloom(),
		StateRoot:        block.Root(),
		Miner:            block.Coinbase(),
		Difficulty:       (*hexutil.Big)(block.Difficulty()),
		ExtraData:        hexutil.Bytes(block.Extra()),
		GasLimit:         hexutil.Uint64(block.GasLimit()),
		GasUsed:          hexutil.Uint64(block.GasUsed()),
		Timestamp:        hexutil.Uint64(block.Time()),
		TransactionsRoot: block.TxHash(),
		ReceiptsRoot:     block.ReceiptHash(),
	}
	if block.Header().BaseFee != nil {
		blockHeader.BaseFeePerGas = (*hexutil.Big)(block.Header().BaseFee)
	}
	if block.Header().WithdrawalsHash != nil {
		blockHeader.WithdrawalsRoot = block.Header().WithdrawalsHash
	}
	if block.Header().BlobGasUsed != nil {
		blockHeader.BlobGasUsed = (*hexutil.Uint64)(block.Header().BlobGasUsed)
	}
	if block.Header().ExcessBlobGas != nil {
		blockHeader.ExcessBlobGas = (*hexutil.Uint64)(block.Header().ExcessBlobGas)
	}
	if block.Header().ParentBeaconRoot != nil {
		blockHeader.ParentBeaconBlockRoot = block.Header().ParentBeaconRoot
	}
	if block.Header().RequestsHash != nil {
		blockHeader.RequestsRoot = block.Header().RequestsHash
	}
	return &blockHeader
}

func BuildPipelineTransaction(tx *types.Transaction, receipt *types.Receipt, from common.Address, baseFee *big.Int) ptypes.Transaction {
	to := receipt.ContractAddress
	if tx.To() != nil {
		to = *tx.To()
	}
	gasPrice := receipt.EffectiveGasPrice
	if gasPrice == nil {
		gasPrice = tx.GasPrice()
	}
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
		TransactionIndex: int64(receipt.TransactionIndex),
		Value:            (*hexutil.Big)(tx.Value()),
	}
	switch tx.Type() {
	case types.DynamicFeeTxType, types.BlobTxType, types.SetCodeTxType:
		transaction.GasFeeCap = tx.GasFeeCap()
		transaction.GasTipCap = tx.GasTipCap()
	}
	return transaction
}
