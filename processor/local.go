package processor

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/DeBankDeFi/pipeline/types"
	"github.com/DeBankDeFi/pipeline/util"
	"github.com/ethereum/go-ethereum/common"
)

type DataFile struct {
	S3key string
	Path  string
	Data  []byte
}

type LocalProcessor struct {
	LocalDirectory string
	Cache          map[common.Hash][]DataFile
	sync.Mutex
}

func NewLocalProcessor(localDirectory string) (*LocalProcessor, error) {
	// check if localDirectory exists
	// if not, create it
	// else return error
	if _, err := os.Stat(localDirectory); os.IsNotExist(err) {
		err = os.MkdirAll(localDirectory, 0755)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("local directory %s does not exist", localDirectory)
	}

	return &LocalProcessor{
		LocalDirectory: localDirectory,
	}, nil
}

func (p *LocalProcessor) GetBlockData(blockHash common.Hash) ([]DataFile, error) {
	p.Lock()
	if data, ok := p.Cache[blockHash]; ok {
		return data, nil
	}
	p.Unlock()
	dataFiles, err := p.loadBlockData(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load block data: %s", err)
	}
	p.Lock()
	defer p.Unlock()
	p.Cache[blockHash] = dataFiles
	return dataFiles, nil
}

// read LocalDirectory blockHash的所有文件，根据path生成所有block,tx,receipt,event,trace和statediff
func (p *LocalProcessor) loadBlockData(blockHash common.Hash) ([]DataFile, error) {
	var dataFiles []DataFile
	// read block
	block, err := p.loadBlock(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load block: %s", err)
	}
	dataFiles = append(dataFiles, block)

	transactions, err := p.loadTransactions(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load transactions: %s", err)
	}
	dataFiles = append(dataFiles, transactions...)

	receipts, err := p.loadReceipts(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load receipts: %s", err)
	}
	dataFiles = append(dataFiles, receipts...)

	traces, err := p.loadTraces(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load traces: %s", err)
	}
	dataFiles = append(dataFiles, traces...)

	events, err := p.loadEvents(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %s", err)
	}
	dataFiles = append(dataFiles, events...)

	stateDiff, err := p.loadStateDiff(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load state diff: %s", err)
	}
	dataFiles = append(dataFiles, stateDiff)
	return dataFiles, nil
}

// path: <localDirectory>/<blockHash>/block.json.gz
// s3Key: block/<blockHash>
func (p *LocalProcessor) ProcessBlock(block *types.Block) error {
	data, err := util.EncodeToJsonGzip(block)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/%s/block.json.gz", p.LocalDirectory, block.Hash.Hex())
	err = util.WriteFile(path, data)
	if err != nil {
		return err
	}
	s3Key := fmt.Sprintf("block/%s", block.Hash.Hex())
	p.Lock()
	defer p.Unlock()
	p.Cache[block.Hash] = []DataFile{{
		S3key: s3Key,
		Path:  path,
		Data:  data,
	}}
	return nil
}

// load block from local directory
func (p *LocalProcessor) loadBlock(blockHash common.Hash) (DataFile, error) {
	path := fmt.Sprintf("%s/%s/block.json.gz", p.LocalDirectory, blockHash.Hex())
	data, err := util.ReadFile(path)
	if err != nil {
		return DataFile{}, fmt.Errorf("failed to read block file: %s", err)
	}
	return DataFile{
		Path: path,
		Data: data,
	}, nil
}

// path: <localDirectory>/<blockHash>/transaction/<transactionHash>.json.gz
// s3Key: transaction/<transactionHash>
func (p *LocalProcessor) ProcessTransactions(transactions []types.Transaction) error {
	for _, transaction := range transactions {
		data, err := util.EncodeToJsonGzip(transactions)
		if err != nil {
			return err
		}
		path := fmt.Sprintf("%s/%s/transaction/%s.json.gz", p.LocalDirectory, transaction.BlockHash.Hex(), transaction.Hash.Hex())
		err = util.WriteFile(path, data)
		if err != nil {
			return err
		}
		s3Key := fmt.Sprintf("transaction/%s/", transaction.Hash.Hex())
		p.Lock()
		p.Cache[*transaction.BlockHash] = append(p.Cache[*transaction.BlockHash], DataFile{
			S3key: s3Key,
			Path:  path,
			Data:  data,
		})
		p.Unlock()
	}
	return nil
}

func (p *LocalProcessor) loadTransactions(blockHash common.Hash) ([]DataFile, error) {
	path := fmt.Sprintf("%s/%s/transaction", p.LocalDirectory, blockHash.Hex())
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	dataFiles := []DataFile{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		transactionHash := strings.Split(file.Name(), ".")[0]
		sufix := strings.Split(file.Name(), ".")[1]
		if sufix != "json.gz" {
			continue
		}
		data, err := util.ReadFile(fmt.Sprintf("%s/%s", path, file.Name()))
		if err != nil {
			return nil, err
		}
		dataFiles = append(dataFiles, DataFile{
			S3key: fmt.Sprintf("transaction/%s/%s", blockHash.Hex(), transactionHash),
			Path:  fmt.Sprintf("%s/%s", path, file.Name()),
			Data:  data,
		})
	}
	return dataFiles, nil
}

// path: <localDirectory>/<blockHash>/receipt/<transactionHash>.json.gz
// s3Key: receipt/<transactionHash>
func (p *LocalProcessor) ProcessReceipts(receipts []types.Receipt) error {
	for _, receipt := range receipts {
		data, err := util.EncodeToJsonGzip(receipts)
		if err != nil {
			return err
		}
		path := fmt.Sprintf("%s/%s/receipt/%s.json.gz", p.LocalDirectory, receipt.BlockHash.Hex(), receipt.TransactionHash.Hex())
		err = util.WriteFile(path, data)
		if err != nil {
			return err
		}
		s3Key := fmt.Sprintf("receipt/%s", receipt.TransactionHash.Hex())
		p.Lock()
		p.Cache[receipt.BlockHash] = append(p.Cache[receipt.BlockHash], DataFile{
			S3key: s3Key,
			Path:  path,
			Data:  data,
		})
		p.Unlock()
	}
	return nil
}

func (p *LocalProcessor) loadReceipts(blockHash common.Hash) ([]DataFile, error) {
	path := fmt.Sprintf("%s/%s/receipt", p.LocalDirectory, blockHash.Hex())
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	dataFiles := []DataFile{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		transactionHash := strings.Split(file.Name(), ".")[0]
		sufix := strings.Split(file.Name(), ".")[1]
		if sufix != "json.gz" {
			continue
		}
		data, err := util.ReadFile(fmt.Sprintf("%s/%s", path, file.Name()))
		if err != nil {
			return nil, err
		}
		dataFiles = append(dataFiles, DataFile{
			S3key: fmt.Sprintf("receipt/%s/%s", blockHash.Hex(), transactionHash),
			Path:  fmt.Sprintf("%s/%s", path, file.Name()),
			Data:  data,
		})
	}
	return dataFiles, nil
}

// path: <localDirectory>/<blockHash>/trace/<transactionHash>/<traceHash>.json.gz
// s3Key: trace/<transactionHash>/<traceHash>
func (p *LocalProcessor) ProcessTraces(traces []types.CallFrame) error {
	for _, trace := range traces {
		data, err := util.EncodeToJsonGzip(traces)
		if err != nil {
			return err
		}
		trace_id := trace.Hash()
		path := fmt.Sprintf("%s/%s/trace/%s/%s.json.gz", p.LocalDirectory, trace.BlockHash.Hex(), trace.TransactionHash.Hex(), trace_id.Hex())
		err = util.WriteFile(path, data)
		if err != nil {
			return err
		}
		s3Key := fmt.Sprintf("trace/%s/%s", trace.TransactionHash.Hex(), trace_id.Hex())
		p.Lock()
		p.Cache[*trace.BlockHash] = append(p.Cache[*trace.BlockHash], DataFile{
			S3key: s3Key,
			Path:  path,
			Data:  data,
		})
		p.Unlock()
	}
	return nil
}

func (p *LocalProcessor) loadTraces(blockHash common.Hash) ([]DataFile, error) {
	path := fmt.Sprintf("%s/%s/trace", p.LocalDirectory, blockHash.Hex())
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	dataFiles := []DataFile{}
	for _, file := range files {
		if file.IsDir() {
			transactionHash := file.Name()
			files, err := os.ReadDir(fmt.Sprintf("%s/%s", path, transactionHash))
			if err != nil {
				return nil, err
			}
			for _, file := range files {
				if file.IsDir() {
					continue
				}
				traceHash := strings.Split(file.Name(), ".")[0]
				sufix := strings.Split(file.Name(), ".")[1]
				if sufix != "json.gz" {
					continue
				}
				data, err := util.ReadFile(fmt.Sprintf("%s/%s/%s", path, transactionHash, file.Name()))
				if err != nil {
					return nil, err
				}
				dataFiles = append(dataFiles, DataFile{
					S3key: fmt.Sprintf("trace/%s/%s", transactionHash, traceHash),
					Path:  fmt.Sprintf("%s/%s/%s", path, transactionHash, file.Name()),
					Data:  data,
				})
			}
		}
	}
	return dataFiles, nil
}

// path: <localDirectory>/<blockHash>/event/<transactionHash>/<eventHash>.json.gz
// s3Key: event/<transactionHash>/<eventHash>
func (p *LocalProcessor) ProcessEvents(blockHash common.Hash, events []types.Event) error {
	for _, event := range events {
		data, err := util.EncodeToJsonGzip(events)
		if err != nil {
			return err
		}
		path := fmt.Sprintf("%s/%s/event/%s/%s.json.gz", p.LocalDirectory, event.BlockHash.Hex(), event.TxHash.Hex(), event.Hash().Hex())
		err = util.WriteFile(path, data)
		if err != nil {
			return err
		}
		s3Key := fmt.Sprintf("event/%s/%s", event.TxHash.Hex(), event.Hash().Hex())
		p.Lock()
		p.Cache[event.BlockHash] = append(p.Cache[event.BlockHash], DataFile{
			S3key: s3Key,
			Path:  path,
			Data:  data,
		})
		p.Unlock()
	}
	return nil
}

func (p *LocalProcessor) loadEvents(blockHash common.Hash) ([]DataFile, error) {
	path := fmt.Sprintf("%s/%s/event", p.LocalDirectory, blockHash.Hex())
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	dataFiles := []DataFile{}
	for _, file := range files {
		if file.IsDir() {
			transactionHash := file.Name()
			files, err := os.ReadDir(fmt.Sprintf("%s/%s", path, transactionHash))
			if err != nil {
				return nil, err
			}
			for _, file := range files {
				if file.IsDir() {
					continue
				}
				eventHash := strings.Split(file.Name(), ".")[0]
				sufix := strings.Split(file.Name(), ".")[1]
				if sufix != "json.gz" {
					continue
				}
				data, err := util.ReadFile(fmt.Sprintf("%s/%s/%s", path, transactionHash, file.Name()))
				if err != nil {
					return nil, err
				}
				dataFiles = append(dataFiles, DataFile{
					S3key: fmt.Sprintf("event/%s/%s", transactionHash, eventHash),
					Path:  fmt.Sprintf("%s/%s/%s", path, transactionHash, file.Name()),
					Data:  data,
				})
			}
		}
	}
	return dataFiles, nil
}

// path: <localDirectory>/<blockHash>/stateDiff.rlp
// s3Key: stateDiff/<blockHash>
func (p *LocalProcessor) ProcessStateDiff(blockHash common.Hash, stateDiff *types.BlockStorageDiff) error {
	data, err := util.EncodeToRlp(stateDiff)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/%s/stateDiff.rlp", p.LocalDirectory, blockHash.Hex())
	err = util.WriteFile(path, data)
	if err != nil {
		return err
	}
	s3Key := fmt.Sprintf("stateDiff/%s", blockHash.Hex())
	p.Lock()
	p.Cache[blockHash] = append(p.Cache[blockHash], DataFile{
		S3key: s3Key,
		Path:  path,
		Data:  data,
	})
	p.Unlock()
	return nil
}

func (p *LocalProcessor) loadStateDiff(blockHash common.Hash) (DataFile, error) {
	path := fmt.Sprintf("%s/%s/stateDiff.rlp", p.LocalDirectory, blockHash.Hex())
	data, err := util.ReadFile(path)
	if err != nil {
		return DataFile{}, err
	}
	return DataFile{
		S3key: fmt.Sprintf("stateDiff/%s", blockHash.Hex()),
		Path:  path,
		Data:  data,
	}, nil
}

// block存在且block statediff存在
func (p *LocalProcessor) CheckBlockDataExist(blockHash common.Hash) (bool, error) {
	blockDataPath := fmt.Sprintf("%s/%s/block.json.gz", p.LocalDirectory, blockHash.Hex())
	if _, err := os.Stat(blockDataPath); err != nil {
		return false, nil
	}
	statediffPath := fmt.Sprintf("%s/%s/stateDiff.rlp", p.LocalDirectory, blockHash.Hex())
	if _, err := os.Stat(statediffPath); err != nil {
		return false, nil
	}
	return true, nil
}
