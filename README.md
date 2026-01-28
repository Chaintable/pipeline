# Pipeline

A blockchain data processing pipeline for extracting, tracing, and processing Ethereum blockchain data (blocks, transactions, state changes, call traces, events), with uploads to S3 and Kafka publishing.

## Features

- Real-time block and transaction tracing via EVM hooks
- State diff tracking (balance, nonce, code, storage changes)
- Call stack tracing for all internal transactions
- Dual S3 bucket strategy for internal and external data
- Kafka notifications for block changes
- Leader election for high availability (manual or etcd-based)
- Multi-chain support: Ethereum, Scroll, XDC, Nitro (Arbitrum), L2geth

## Quick Start

### Prerequisites

- Go 1.23+
- AWS credentials configured
- Kafka cluster (optional)
- etcd cluster (optional, for leader election)

### Build

```bash
go build ./...
```

### Run Tests

```bash
# All tests
go test ./...

# Specific package
go test -v ./types/...
go test -v ./util/...
```

### Integration Modes

Pipeline supports two integration modes:

**Mode 1: Live Tracer** - Real-time tracing with automatic S3/Kafka upload

```
Block Execution → PipelineTracer → S3 Upload + Kafka Publish
```

**Mode 2: RPC Tracer** - On-demand tracing via `trace_debankBlock` RPC

```
RPC Request → Block Replay → RPCTracer → Return DebankOutPut
```

See [docs/integration-modes.md](docs/integration-modes.md) for detailed documentation.

### Mode 1: Live Tracer

Embeds tracer into block processing, automatically uploads to S3 and publishes to Kafka.

```go
import "github.com/Chaintable/pipeline/tracer"

// Initialize pipeline (S3 + Kafka)
tracer.InitPipeline(
    region,           // AWS region
    nodeXBucket,      // Internal S3 bucket
    chainTableBucket, // External S3 bucket
    brokers,          // Kafka brokers
    topic,            // Kafka topic
    bizChainID,       // Chain ID
    version,          // Version
    s3TmpDir,         // Local temp directory for S3 uploads
)

// Optional: Setup leader election
tracer.SetupLeaderElection(
    etcdEndpoints,    // etcd endpoints
    electionKey,      // Election key path
    nodeID,           // Unique node ID
    version,          // Version
    isBackup,         // nil for auto election, true/false for manual
    gracePeriod,      // Grace period for leader transition
    writerConfig,     // Writer registry config (optional)
)

// Create tracer and register with EVM
pipelineTracer := tracer.NewPipelineTracer(configJSON)
// Register hooks: OnBlockStart, OnTxStart, OnCommit, etc.
```

### Mode 2: RPC Tracer (trace_debankBlock)

Implements `trace_debankBlock` RPC method, returns `DebankOutPut` containing BlockFile, Header, StateDiff.

```go
import "github.com/Chaintable/pipeline/tracer"

// Implement RPC method
func (api *DebugAPI) TraceDebankBlock(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.DebankOutPut, error) {
    // Create RPC tracer
    rpcTracer := tracer.NewRPCTracer(configJSON)

    // Replay block with tracer hooks
    // ...

    // Get complete output
    return rpcTracer.GetOutPut(originRoot, root, destructs, accounts, storages, codes)
}

// DebankOutPut contains:
// - BlockFile: block, transactions, traces, events
// - Header: full block header
// - StateDiff: RLP-encoded state changes
// - ValidationHash: integrity checksum
```

## Project Structure

```
pipeline/
├── tracer/       # Block/transaction tracing (EVM hooks)
├── processor/    # Data serialization and upload
├── types/        # Core data structures
├── leader/       # Leader election (manual/etcd)
├── writer/       # Writer node registration
├── util/         # S3, Kafka, codec utilities
└── metrics/      # Observability metrics
```

## Data Flow

```
Ethereum Node
    ↓
Pipeline Tracer (extract blocks/transactions/state)
    ↓
Call/PreState Tracers (detailed tracing)
    ↓
Processor (serialize to JSON/gzip + RLP)
    ↓
S3 Upload (dual bucket) + Kafka Publish (BlockChangeNotification)
```

## Dependencies

- [go-ethereum](https://github.com/ethereum/go-ethereum) v1.15.11 - Ethereum core types
- [etcd client](https://github.com/etcd-io/etcd) v3.5.10 - Leader election
- [AWS SDK v2](https://github.com/aws/aws-sdk-go-v2) - S3 operations
- [kafka-go](https://github.com/segmentio/kafka-go) - Kafka publishing

## License

[MIT](LICENSE)
