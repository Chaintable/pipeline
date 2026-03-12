package tracer

import (
	"fmt"
	"math/big"
	"time"

	"github.com/Chaintable/pipeline/leader"
	"github.com/Chaintable/pipeline/metrics"
	"github.com/Chaintable/pipeline/processor"
	ptypes "github.com/Chaintable/pipeline/types"
	"github.com/Chaintable/pipeline/writer"
	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/core"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/crypto"
	"github.com/ethereum-optimism/optimism/l2geth/log"
	"github.com/ethereum-optimism/optimism/l2geth/rlp"
	"github.com/holiman/uint256"
)

type ExtraInfo struct {
	BlockNumber     uint64
	BlockHash       common.Hash
	BlockFile       *ptypes.BlockFile
	Tx              *types.Transaction
	From            common.Address
	BlockHeader     *ptypes.Header
	BlockDiff       *ptypes.BlockStorageDiff
	BlockChange     *ptypes.BlockChangeNotification
	Committed       bool
	ChangeContracts map[common.Address]struct{}
	// metrics timer
	TxStartTime    time.Time
	BlockStartTime time.Time
}

var (
	NodeXPusher            *processor.PushProcessor
	ChainTableBucketPusher *processor.PushProcessor
	BlockCtx               *ExtraInfo
	BizChainID             string
	Version                string
	LeaderManager          *leader.Manager
	WriterRegistry         *writer.WriterRegistry
)

func InitPipeline(region string, nodeXBucket string, chainTableBucket string, brokers []string, topic string, bizChainID string, version string, s3TmpDir string) (err error) {
	// Create processors
	NodeXPusher, err = processor.NewPushProcessor(region, nodeXBucket, brokers, topic, s3TmpDir)
	if err != nil {
		return err
	}
	ChainTableBucketPusher, err = processor.NewPushProcessor(region, chainTableBucket, brokers, topic, s3TmpDir)
	if err != nil {
		return err
	}

	BizChainID = bizChainID
	Version = version
	return nil
}

// WriterRegistryConfig holds configuration for writer node registration
type WriterRegistryConfig struct {
	TTL              int64
	NodeXBucket      string
	ChainTableBucket string
	Region           string
	Brokers          []string
	Topic            string
}

// SetupLeaderElection sets up manual leader election for the processors
func SetupLeaderElection(etcdEndpoints []string, electionKey string, nodeID string, version string, isBackup *bool, gracePeriod int, writerConfig *WriterRegistryConfig) error {
	// Create a single leader manager for both processors
	config := leader.ManagerConfig{
		EtcdEndpoints: etcdEndpoints,
		ElectionKey:   electionKey,
		NodeID:        nodeID,
		IsBackup:      isBackup,
		GracePeriod:   time.Duration(gracePeriod) * time.Second,
		OnBecomeLeader: func() error {
			// Update last block when becoming leader
			log.Info("Updating last block info on leader transition")
			if NodeXPusher != nil {
				if err := NodeXPusher.UpdateLastBlock(); err != nil {
					log.Error("Failed to update NodeX last block", "err", err)
				}
			}
			if ChainTableBucketPusher != nil {
				if err := ChainTableBucketPusher.UpdateLastBlock(); err != nil {
					log.Error("Failed to update ChainTable last block", "err", err)
				}
			}
			return nil
		},
		OnLoseLeader: func() error {
			return nil
		},
	}

	var err error
	leader.GlobalManager, err = leader.NewManager(&config)
	if err != nil {
		return fmt.Errorf("failed to create leader manager: %w", err)
	}

	// Initialize writer registry in failover mode
	if writerConfig != nil {
		// Use the same etcd client from leader manager
		etcdClient := leader.GlobalManager.GetEtcdClient()

		// Create writer node info
		nodeInfo := writer.WriterNodeInfo{
			NodeXBucket:      writerConfig.NodeXBucket,
			ChainTableBucket: writerConfig.ChainTableBucket,
			Region:           writerConfig.Region,
			Brokers:          writerConfig.Brokers,
			Topic:            writerConfig.Topic,
		}

		WriterRegistry = writer.NewWriterRegistry(etcdClient, BizChainID, nodeID, version, nodeInfo, writerConfig.TTL)

		// Register node immediately when initialized (not waiting to become leader)
		if err := WriterRegistry.RegisterNode(); err != nil {
			log.Error("Failed to register writer node during initialization", "err", err)
		} else {
			log.Info("Writer node registered during initialization", "chainID", BizChainID, "nodeID", nodeID)
		}
	}

	if err := leader.GlobalManager.Start(); err != nil {
		return fmt.Errorf("failed to start leader manager: %w", err)
	}

	log.Info("Leader election setup completed", "nodeID", nodeID, "electionKey", electionKey)

	return nil
}

// decodeSlimAccount decodes RLP-encoded slim account data
// Replaces snapshot.FullAccount() which is not available in l2geth
func decodeSlimAccount(data []byte) (nonce uint64, balance *big.Int, root common.Hash, codeHash []byte, err error) {
	var account struct {
		Nonce    uint64
		Balance  *big.Int
		Root     []byte
		CodeHash []byte
	}
	if err := rlp.DecodeBytes(data, &account); err != nil {
		return 0, nil, common.Hash{}, nil, err
	}

	// Handle empty values
	if len(account.Root) == 0 {
		root = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421") // emptyRoot
	} else {
		root = common.BytesToHash(account.Root)
	}

	if len(account.CodeHash) == 0 {
		codeHash = crypto.Keccak256(nil) // emptyCode
	} else {
		codeHash = account.CodeHash
	}

	return account.Nonce, account.Balance, root, codeHash, nil
}

func stateUpdateToStateDiff(originRoot common.Hash, root common.Hash, destructs map[common.Hash]struct{}, accounts map[common.Hash][]byte, storages map[common.Hash]map[common.Hash][]byte, codes map[common.Hash][]byte) *ptypes.BlockStorageDiff {
	stateDiff := &ptypes.BlockStorageDiff{}
	for addrhash := range destructs {
		stateDiff.DeletedAccounts = append(stateDiff.DeletedAccounts, addrhash)
	}
	for k, v := range accounts {
		nonce, balance, _, codeHash, err := decodeSlimAccount(v)
		if err != nil {
			log.Error("Failed to decode slim account", "err", err)
			continue
		}
		stateDiff.NewAccounts = append(stateDiff.NewAccounts, ptypes.NewAccount{
			Address:  k,
			Balance:  uint256.MustFromBig(balance),
			Nonce:    nonce,
			CodeHash: common.BytesToHash(codeHash),
		})
	}
	for account, storage := range storages {
		Values := make([]ptypes.IndexValuePair, 0, len(storage))
		for index, v := range storage {
			value := uint256.NewInt(0)
			if len(v) > 0 {
				_, content, _, err := rlp.Split(v)
				if err != nil {
					log.Error("Failed to split storage", "err", err)
				}
				value = uint256.NewInt(0).SetBytes(content)
			}
			Values = append(Values, ptypes.IndexValuePair{
				Index: index,
				Value: value,
			})
		}
		stateDiff.StorageDiff = append(stateDiff.StorageDiff, ptypes.AccountStorageDiff{
			Address: account,
			Values:  Values,
		})
	}
	for hash, code := range codes {
		stateDiff.NewCodes = append(stateDiff.NewCodes, ptypes.NewCode{
			CodeHash: hash,
			Code:     code,
		})
	}
	if originRoot == (common.Hash{}) {
		originRoot = types.EmptyRootHash
	}
	if root == (common.Hash{}) {
		root = types.EmptyRootHash
	}
	stateDiff.Hash = root
	stateDiff.ParentHash = originRoot
	return stateDiff
}

func GenesisAllocToStateDiff(genesisAlloc core.GenesisAlloc) *ptypes.BlockStorageDiff {
	diff := &ptypes.BlockStorageDiff{}
	diff.NewAccounts = make([]ptypes.NewAccount, 0)
	diff.NewCodes = make([]ptypes.NewCode, 0)
	diff.StorageDiff = make([]ptypes.AccountStorageDiff, 0)
	diff.DeletedAccounts = make([]common.Hash, 0)
	for addr, acc := range genesisAlloc {
		diff.NewAccounts = append(diff.NewAccounts, ptypes.NewAccount{
			Address:  crypto.Keccak256Hash(addr[:]),
			Balance:  uint256.MustFromBig(acc.Balance),
			Nonce:    acc.Nonce,
			CodeHash: crypto.Keccak256Hash(acc.Code),
		})
		if len(acc.Code) > 0 {
			diff.NewCodes = append(diff.NewCodes, ptypes.NewCode{
				CodeHash: crypto.Keccak256Hash(acc.Code),
				Code:     acc.Code,
			})
		}
		values := make([]ptypes.IndexValuePair, 0)
		for index, v := range acc.Storage {
			value := uint256.NewInt(0)
			if len(v) > 0 {
				value = uint256.NewInt(0).SetBytes(v.Bytes())
			}
			values = append(values, ptypes.IndexValuePair{
				Index: crypto.Keccak256Hash(index[:]),
				Value: value,
			})
		}
		diff.StorageDiff = append(diff.StorageDiff, ptypes.AccountStorageDiff{
			Address: crypto.Keccak256Hash(addr[:]),
			Values:  values,
		})
	}
	return diff
}

func uploadBlockHeader(blockHeader *ptypes.Header) error {
	start := time.Now()
	defer func() {
		metrics.BlockHeaderUploadTimer.UpdateSince(start)
	}()
	s3BlockFile, err := processor.SerializeHeader(BizChainID, Version, blockHeader)
	if err != nil {
		return fmt.Errorf("failed to serialize block header: %v", err)
	}
	err = NodeXPusher.UploadFile(s3BlockFile)
	if err != nil {
		return fmt.Errorf("failed to upload block header: %v", err)
	}
	return nil
}

func uploadBlockDiff(blockDiff *ptypes.BlockStorageDiff) error {
	start := time.Now()
	defer func() {
		metrics.StateDiffUploadTimer.UpdateSince(start)
	}()
	s3file, err := processor.SerializeStateDiff(BizChainID, Version, blockDiff)
	if err != nil {
		return fmt.Errorf("failed to serialize state diff: %v", err)
	}
	err = NodeXPusher.UploadFile(s3file)
	if err != nil {
		return fmt.Errorf("failed to upload state diff: %v", err)
	}
	return nil
}

func uploadBlockFile(blockFile *ptypes.BlockFile) error {
	s3file, err := processor.SerializeFile(BizChainID, Version, blockFile)
	if err != nil {
		return fmt.Errorf("failed to serialize block file: %v", err)
	}
	err = ChainTableBucketPusher.UploadFile(s3file)
	if err != nil {
		return fmt.Errorf("failed to upload block file: %v", err)
	}
	return nil
}

func uploadblockFileValidation(blockFile *ptypes.BlockFile) error {
	start := time.Now()
	defer func() {
		metrics.BlockFileValidationTimer.UpdateSince(start)
	}()
	blockFileValidation, err := processor.SerializeFileValidation(BizChainID, Version, blockFile)
	if err != nil {
		return fmt.Errorf("failed to serialize block file validation: %v", err)
	}
	err = ChainTableBucketPusher.UploadFile(blockFileValidation)
	if err != nil {
		return fmt.Errorf("failed to upload block file validation: %v", err)
	}
	return nil
}
