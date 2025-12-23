package util

import (
	"encoding/hex"
	"log"
	"math/big"
	"strings"
	"time"

	ptypes "github.com/Chaintable/pipeline/types"
	lru "github.com/hashicorp/golang-lru"
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/common/hexutil"
	"github.com/kaiachain/kaia/consensus/istanbul"
	"github.com/kaiachain/kaia/crypto/sha3"
	"github.com/kaiachain/kaia/params"
	"github.com/kaiachain/kaia/rlp"
)

var (
	inmemoryBlocks             = 2048 // Number of blocks to precompute validators' addresses
	inmemoryValidatorsPerBlock = 30   // Approximate number of validators' addresses from ecrecover
	signatureAddresses, _      = lru.NewARC(inmemoryBlocks * inmemoryValidatorsPerBlock)
)

func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	// Clean seal is required for calculating proposer seal.
	rlp.Encode(hasher, types.IstanbulFilteredHeader(header, false))
	hasher.Sum(hash[:0])
	return hash
}

// cacheSignatureAddresses extracts the address from the given data and signature and cache them for later usage.
func cacheSignatureAddresses(data []byte, sig []byte) (common.Address, error) {
	sigStr := hex.EncodeToString(sig)
	if addr, ok := signatureAddresses.Get(sigStr); ok {
		return addr.(common.Address), nil
	}
	addr, err := istanbul.GetSignatureAddress(data, sig)
	if err != nil {
		return common.Address{}, err
	}
	signatureAddresses.Add(sigStr, addr)
	return addr, err
}

// ecrecover extracts the Kaia account address from a signed header.
func ecrecover(header *types.Header) (common.Address, error) {
	// Retrieve the signature from the header extra-data
	istanbulExtra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return common.Address{}, err
	}
	addr, err := cacheSignatureAddresses(sigHash(header).Bytes(), istanbulExtra.Seal)
	if err != nil {
		return addr, err
	}

	return addr, nil
}

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
	miner, err := ecrecover(block.Header())
	if err != nil {
		log.Println("Failed to ecrecover miner", "err", err)
		miner = common.Address{}
	}
	blockHeader := ptypes.Header{
		Number:     (*hexutil.Big)(block.Number()),
		Hash:       block.Hash(),
		ParentHash: block.ParentHash(),
		Nonce:      "0x00000000",
		MixHash:    common.BytesToHash(block.Header().MixHash),
		Sha3Uncles: common.HexToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347"), // always the empty hash for klay
		LogsBloom:  block.Bloom(),
		StateRoot:  block.Root(),
		Miner:      miner,
		Difficulty: (*hexutil.Big)(block.Header().BlockScore),
		// Copy from Kaia source code: https://github.com/kaiachain/kaia/api/api_eth.go#L1209
		// extraData always return empty Bytes because actual value of extraData in Kaia header cannot be used as meaningful way because
		// we cannot provide original header of Kaia and this field is used as consensus info which is encoded value of validators addresses, validators signatures, and proposer signature in Kaia.
		ExtraData:        hexutil.Bytes{}, // hexutil.Bytes(block.Extra())
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
