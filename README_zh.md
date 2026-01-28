# Pipeline

一个通用区块链节点数据处理管线，用于提取、追踪和处理以太坊区块链数据（区块、交易、状态变更、调用追踪、事件），并上传至 S3 和 Kafka。

## 功能特性

- 通过 EVM 钩子实时追踪区块和交易
- 状态差异追踪（余额、Nonce、代码、存储变更）
- 所有内部交易的调用栈追踪
- 双 S3 桶策略，分离内部和外部数据
- Kafka 区块变更通知
- 高可用领导者选举（手动模式或基于 etcd）
- 多链支持：Ethereum、Scroll、XDC、Nitro (Arbitrum)、L2geth

## 快速开始

### 前置条件

- Go 1.23+
- 已配置 AWS 凭证
- Kafka 集群（可选）
- etcd 集群（可选，用于领导者选举）

### 构建

```bash
go build ./...
```

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行单个包的测试
go test -v ./types/...
go test -v ./util/...
```

### 集成模式

Pipeline 支持两种集成模式：

**模式 1：Live Tracer** - 实时追踪，自动上传 S3 和发布 Kafka

```
区块执行 → PipelineTracer → S3 上传 + Kafka 发布
```

**模式 2：RPC Tracer** - 按需追踪，通过 `trace_debankBlock` RPC 接口

```
RPC 请求 → 区块重放 → RPCTracer → 返回 DebankOutPut
```

详细文档请参阅 [docs/integration-modes.md](docs/integration-modes.md)。

### 模式 1：Live Tracer

将追踪器嵌入区块处理流程，自动上传数据到 S3 并发布 Kafka 通知。

```go
import "github.com/Chaintable/pipeline/tracer"

// 初始化管线（S3 + Kafka）
tracer.InitPipeline(
    region,           // AWS 区域
    nodeXBucket,      // 内部 S3 桶
    chainTableBucket, // 外部 S3 桶
    brokers,          // Kafka brokers
    topic,            // Kafka topic
    bizChainID,       // 链 ID
    version,          // 版本
    s3TmpDir,         // S3 上传本地临时目录
)

// 可选：设置领导者选举
tracer.SetupLeaderElection(
    etcdEndpoints,    // etcd 端点
    electionKey,      // 选举键路径
    nodeID,           // 唯一节点 ID
    version,          // 版本
    isBackup,         // nil=自动选举，true/false=手动模式
    gracePeriod,      // Leader 切换宽限期
    writerConfig,     // Writer 注册配置（可选）
)

// 创建追踪器并注册到 EVM
pipelineTracer := tracer.NewPipelineTracer(configJSON)
// 注册钩子：OnBlockStart, OnTxStart, OnCommit 等
```

### 模式 2：RPC Tracer (trace_debankBlock)

实现 `trace_debankBlock` RPC 方法，返回包含 BlockFile、Header、StateDiff 的 `DebankOutPut`。

```go
import "github.com/Chaintable/pipeline/tracer"

// 实现 RPC 方法
func (api *DebugAPI) TraceDebankBlock(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.DebankOutPut, error) {
    // 创建 RPC 追踪器
    rpcTracer := tracer.NewRPCTracer(configJSON)

    // 使用追踪器钩子重放区块
    // ...

    // 获取完整输出
    return rpcTracer.GetOutPut(originRoot, root, destructs, accounts, storages, codes)
}

// DebankOutPut 包含：
// - BlockFile：区块、交易、追踪、事件
// - Header：完整区块头
// - StateDiff：RLP 编码的状态变更
// - ValidationHash：完整性校验哈希
```

## 项目结构

```
pipeline/
├── tracer/       # 区块/交易追踪（EVM 钩子）
├── processor/    # 数据序列化和上传
├── types/        # 核心数据结构
├── leader/       # 领导者选举（手动/etcd）
├── writer/       # 写入节点注册
├── util/         # S3、Kafka、编解码工具
└── metrics/      # 可观测性指标
```

## 数据流程

```
以太坊节点
    ↓
Pipeline Tracer（提取区块/交易/状态）
    ↓
Call/PreState Tracers（详细追踪）
    ↓
Processor（序列化为 JSON/gzip + RLP）
    ↓
S3 上传（双桶）+ Kafka 发布（BlockChangeNotification）
```

## 依赖

- [go-ethereum](https://github.com/ethereum/go-ethereum) v1.15.11 - 以太坊核心数据结构
- [etcd client](https://github.com/etcd-io/etcd) v3.5.10 - 领导者选举
- [AWS SDK v2](https://github.com/aws/aws-sdk-go-v2) - S3 操作
- [kafka-go](https://github.com/segmentio/kafka-go) - Kafka 发布

## 许可证

[MIT](LICENSE)
