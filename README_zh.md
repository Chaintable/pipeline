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

**模式 1：Live Tracer**
```
以太坊节点（区块执行）
    ↓
PipelineTracer（EVM 钩子）
    ↓
CallTracer + PrestateTracer（追踪、事件、状态差异）
    ↓
Processor（序列化为 JSON/gzip + RLP）
    ↓
S3 上传（双桶）+ Kafka 发布（BlockChangeNotification）
```

**模式 2：RPC Tracer**
```
RPC 请求（trace_debankBlock）
    ↓
使用 RPCTracer 重放区块
    ↓
CallTracer + PrestateTracer（追踪、事件、状态差异）
    ↓
返回 DebankOutPut（BlockFile + Header + StateDiff + ValidationHash）
```

## 执行客户端适配

将 Pipeline 集成到执行客户端时，请根据客户端类型选择对应的适配指南：

| 指南 | 适用客户端 | 使用场景 |
|------|-----------|----------|
| [标准 Geth 适配](docs/skills/adapt-pipeline-geth/references/adaptation-guide.md) | go-ethereum **v1.14.0+** | 支持 `tracing.Hooks` Live Tracer 的 Geth 及其分叉 |
| [遗留 Geth 适配](docs/skills/adapt-pipeline-legacy/references/adaptation-guide-legacy.md) | go-ethereum **< v1.14.0** | 使用 `vm.EVMLogger` 接口的旧版 Geth 分叉（如 op-geth） |
| [Reth 适配](docs/skills/adapt-pipeline-reth/references/adaptation-guide-reth.md) | Reth (Rust) | Rust 客户端；仅 RPC Tracer 模式，无需修改核心 EVM |

### 如何选择

```
你的客户端是 Rust 编写的吗？
    是 → Reth 适配指南
    否 ↓
你的 Geth 分叉是否有 tracing.Hooks 和 tracers.LiveDirectory？（v1.14.0+）
    是 → 标准适配指南
    否 → 遗留适配指南
```

**核心差异：**

| | 标准 (Geth v1.14.0+) | 遗留 (Geth < v1.14.0) | Reth |
|-|----------------------|----------------------|------|
| Tracer 接口 | `tracing.Hooks` | `vm.EVMLogger` | `revm-inspectors` |
| Pipeline 代码 | Go 模块依赖 | 源码内嵌 | Rust 重新实现 |
| 集成模式 | Live Tracer + RPC | Live Tracer | 仅 RPC |
| 核心 EVM 改动 | Hook 注入 | 手动分发 Hook | 无需改动 |

### 使用 Claude Code AI 辅助适配

如果你使用 [Claude Code](https://claude.ai/code)，可以通过交互式适配技能让 AI 引导你完成整个适配过程——自动检测客户端类型、探测代码结构、生成修改代码、逐步验证编译。

#### 前置条件

1. 安装 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
2. 克隆本 Pipeline 仓库
3. 在本地准备好目标客户端仓库

#### 快速开始

在 Pipeline 仓库目录下打开 Claude Code，运行：

```
/adapt-pipeline /path/to/your-client
```

该命令会：
1. **检测** 客户端类型——扫描 `Cargo.toml`、`go.mod`、`tracing.Hooks`、`vm.EVMLogger` 等特征
2. **报告** 检测结果（语言、客户端类型、检测依据）
3. **检查** 是否已有 Pipeline 集成，避免冲突
4. **路由** 到对应的专用适配技能

#### 示例会话

```
> /adapt-pipeline /home/user/op-geth

## 检测结果
- 仓库: /home/user/op-geth
- 语言: Go
- 客户端类型: 遗留 Geth
- 依据: 在 core/vm/logger.go 中发现 vm.EVMLogger，未找到 core/tracing/hooks.go
- 推荐技能: adapt-pipeline-legacy

正在路由到 /adapt-pipeline-legacy...

## 阶段 1：嵌入 Pipeline 源码
[AI 读取 go.mod，复制 pipeline/ 目录，更新 import 路径...]

## 阶段 2：创建 Tracing Hooks
[AI 创建 core/tracing/hooks.go，定义自定义 hook 类型...]

...每个阶段：探测 → 参考 → 修改 → 验证 (go build)...
```

#### 可用技能

| 技能 | 目标客户端 | 阶段数 | 功能 |
|------|-----------|--------|------|
| `/adapt-pipeline` | 任意 | — | 入口：自动检测客户端类型并路由 |
| `/adapt-pipeline-geth` | Geth v1.14.0+ | 7 | 扩展 `tracing.Hooks`，修改 StateDB/BlockChain，注册 Live Tracer，添加 RPC 接口 |
| `/adapt-pipeline-legacy` | Geth < v1.14.0 | 12 | 嵌入源码，创建 hooks，手动分发，禁用非追踪路径的 tracer |
| `/adapt-pipeline-reth` | Reth (Rust) | 7 | Rust 重新实现类型，添加 `StateDiffTraceDB`，实现 `trace_debankBlock` RPC |

如果你已经知道客户端类型，也可以直接调用专用技能：

```
/adapt-pipeline-geth /path/to/geth-fork
/adapt-pipeline-legacy /path/to/op-geth
/adapt-pipeline-reth /path/to/reth-fork
```

#### 每个阶段的工作方式

每个阶段遵循统一的交互流程：

1. **探测** — AI 读取目标文件，搜索关键模式，理解现有代码结构
2. **参考** — AI 查阅适配指南，获取代码模板和要求
3. **修改** — AI 根据你的代码库生成并应用针对性的修改
4. **验证** — 运行 `go build ./...` 或 `cargo check` 确保编译通过

如果某个阶段验证失败，AI 会分析错误并修复后再继续下一阶段。

## 依赖

- [go-ethereum](https://github.com/ethereum/go-ethereum) v1.15.11 - 以太坊核心数据结构
- [etcd client](https://github.com/etcd-io/etcd) v3.5.10 - 领导者选举
- [AWS SDK v2](https://github.com/aws/aws-sdk-go-v2) - S3 操作
- [kafka-go](https://github.com/segmentio/kafka-go) - Kafka 发布

## 许可证

[MIT](LICENSE)
