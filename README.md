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

**Mode 1: Live Tracer**
```
Ethereum Node (block execution)
    ↓
PipelineTracer (EVM hooks)
    ↓
CallTracer + PrestateTracer (traces, events, state diff)
    ↓
Processor (serialize to JSON/gzip + RLP)
    ↓
S3 Upload (dual bucket) + Kafka Publish (BlockChangeNotification)
```

**Mode 2: RPC Tracer**
```
RPC Request (trace_debankBlock)
    ↓
Block Replay with RPCTracer
    ↓
CallTracer + PrestateTracer (traces, events, state diff)
    ↓
Return DebankOutPut (BlockFile + Header + StateDiff + ValidationHash)
```

## Execution Client Adaptation

To integrate Pipeline into an execution client, follow the guide matching your client type:

| Guide | Client | When to Use |
|-------|--------|-------------|
| [Standard Geth Adaptation](docs/skills/adapt-pipeline-geth/references/adaptation-guide.md) | go-ethereum **v1.14.0+** | Geth and forks with `tracing.Hooks` live tracer support |
| [Legacy Geth Adaptation](docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md) | go-ethereum **< v1.14.0** | Older geth forks using the `vm.EVMLogger` interface (e.g., op-geth) |
| [Reth Adaptation](docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md) | Reth (Rust) | Rust-based clients; RPC Tracer only, no core EVM changes needed |

### Choosing the Right Guide

```
Is your client written in Rust?
    YES → Reth Adaptation Guide
    NO  ↓
Does your geth fork have tracing.Hooks and tracers.LiveDirectory? (v1.14.0+)
    YES → Standard Adaptation Guide
    NO  → Legacy Adaptation Guide
```

**Key differences:**

| | Standard (Geth v1.14.0+) | Legacy (Geth < v1.14.0) | Reth |
|-|--------------------------|-------------------------|------|
| Tracer interface | `tracing.Hooks` | `vm.EVMLogger` | `revm-inspectors` |
| Pipeline code | Go module dependency | Embedded Go source | Reimplemented in Rust |
| Integration mode | Live Tracer + RPC | Live Tracer | RPC only |
| Core EVM changes | Hook injection | Manual hook dispatch | None |

### AI-Assisted Adaptation with Claude Code

If you use [Claude Code](https://claude.ai/code), you can use the interactive adaptation skills to have AI guide you through the entire process — auto-detecting your client type, exploring your codebase, generating modification code, and verifying each step.

#### Prerequisites

1. Install [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
2. Clone this Pipeline repository
3. Have your target client repository ready locally

#### Quick Start

Open Claude Code in the Pipeline repository directory, then run:

```
/adapt-pipeline /path/to/your-client
```

This will:
1. **Detect** your client type by scanning for `Cargo.toml`, `go.mod`, `tracing.Hooks`, `vm.EVMLogger`, etc.
2. **Report** the detection result (language, client type, evidence)
3. **Check** for existing Pipeline integration to avoid conflicts
4. **Route** to the matching specialized skill automatically

#### Example Session

```
> /adapt-pipeline /home/user/op-geth

## Detection Result
- Repository: /home/user/op-geth
- Language: Go
- Client Type: Legacy Geth
- Evidence: Found vm.EVMLogger in core/vm/logger.go, no core/tracing/hooks.go
- Recommended Skill: adapt-pipeline-legacy

Routing to /adapt-pipeline-legacy...

## Phase 1: Embed Pipeline Source Code
[AI reads your go.mod, copies pipeline/ directory, updates import paths...]

## Phase 2: Create Tracing Hooks
[AI creates core/tracing/hooks.go with custom hook types...]

...each phase: explore → reference → modify → verify (go build)...
```

#### Available Skills

| Skill | Target Client | Phases | What It Does |
|-------|--------------|--------|-------------|
| `/adapt-pipeline` | Any | — | Entry point: auto-detects client type and routes |
| `/adapt-pipeline-geth` | Geth v1.14.0+ | 7 | Extends `tracing.Hooks`, modifies StateDB/BlockChain, registers live tracer, adds RPC endpoint |
| `/adapt-pipeline-legacy` | Geth < v1.14.0 | 12 | Embeds pipeline source, creates hooks, manual dispatch, disables tracer in non-tracing paths |
| `/adapt-pipeline-reth` | Reth (Rust) | 7 | Reimplements types in Rust, adds `StateDiffTraceDB`, implements `trace_debankBlock` RPC |

You can also invoke a specialized skill directly if you already know your client type:

```
/adapt-pipeline-geth /path/to/geth-fork
/adapt-pipeline-legacy /path/to/op-geth
/adapt-pipeline-reth /path/to/reth-fork
```

#### How Each Phase Works

Every phase follows the same interactive pattern:

1. **Explore** — AI reads target files, searches for key patterns, understands existing structure
2. **Reference** — AI consults the adaptation guide for code templates and requirements
3. **Modify** — AI generates and applies changes tailored to your codebase
4. **Verify** — Runs `go build ./...` or `cargo check` to ensure compilation passes

If a phase fails verification, AI will analyze the errors and fix them before moving on.

## Dependencies

- [go-ethereum](https://github.com/ethereum/go-ethereum) v1.15.11 - Ethereum core types
- [etcd client](https://github.com/etcd-io/etcd) v3.5.10 - Leader election
- [AWS SDK v2](https://github.com/aws/aws-sdk-go-v2) - S3 operations
- [kafka-go](https://github.com/segmentio/kafka-go) - Kafka publishing

## License

[MIT](LICENSE)
