# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Pipeline 是一个通用区块链节点数据处理管线，用于提取、追踪和处理以太坊区块链数据（区块、交易、状态变更、调用追踪、事件），并上传至 S3 和 Kafka。

## 常用命令

```bash
# 构建
go build ./...

# 运行所有测试
go test ./...

# 运行单个包的测试
go test -v ./types/...
go test -v ./util/...

# 运行特定测试函数
go test -v -run TestFunctionName ./package/...
```

## 架构概览

Pipeline 支持两种集成模式：

**模式 1：Live Tracer** - 实时追踪，自动上传 S3 和发布 Kafka
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

**模式 2：RPC Tracer** - 按需追踪，通过 `trace_debankBlock` RPC 接口
```
RPC 请求（trace_debankBlock）
    ↓
使用 RPCTracer 重放区块
    ↓
CallTracer + PrestateTracer（追踪、事件、状态差异）
    ↓
返回 DebankOutPut（BlockFile + Header + StateDiff + ValidationHash）
```

### 核心模块

- **tracer/**: 区块链数据提取，使用以太坊 tracing 机制
  - `pipeline_tracer.go` - Live Tracer 模式，实时追踪并上传 S3/Kafka
  - `rpc_tracer.go` - RPC Tracer 模式，实现 trace_debankBlock 接口
  - `call_tracer.go` - 交易调用栈追踪
  - `prestate_tracer.go` - 状态差异追踪

- **processor/**: 数据处理和序列化
  - `push.go` - S3 上传、Kafka 发布、重试逻辑
  - `serializer.go` - JSON/gzip 和 RLP 格式转换

- **types/**: 核心数据结构
  - `Block`, `BlockFile`, `Transaction`, `Event`, `Trace`
  - `BlockStorageDiff` - 状态变更
  - `BlockValidation` - 完整性校验

- **leader/**: 分布式领导者选举
  - 固定模式：通过 `isBackup` 手动指定
  - 故障转移模式：基于 etcd 自动选举

- **writer/**: 写入节点注册和发现（etcd）

- **util/**: 通用工具（S3、Kafka、编解码）

- **metrics/**: 可观测性指标

### 数据上传

1. **Block Headers** → 内部 S3 桶 (NodeXBucket)，JSON/gzip 格式
2. **State Diffs** → 内部 S3 桶，RLP 编码
3. **Block Files** → 外部 S3 桶 (ChainTableBucket)，JSON/gzip + 校验和

### 入口点

**模式 1：Live Tracer**
```go
// 初始化管线（S3 + Kafka）
tracer.InitPipeline(region, nodeXBucket, chainTableBucket, brokers, topic, bizChainID, version, s3TmpDir)

// 设置领导者选举（可选）
tracer.SetupLeaderElection(etcdEndpoints, electionKey, nodeID, version, isBackup, gracePeriod, writerConfig)

// 创建追踪器并注册到 EVM
pipelineTracer := tracer.NewPipelineTracer(configJSON)
```

**模式 2：RPC Tracer**
```go
// 实现 trace_debankBlock RPC 方法
rpcTracer := tracer.NewRPCTracer(configJSON)
// 重放区块后获取输出
output := rpcTracer.GetOutPut(originRoot, root, destructs, accounts, storages, codes)
```

### 多链支持

支持多种链变体：标准以太坊、Scroll、XDC、Nitro (Arbitrum)、L2geth

## 关键依赖

- go-ethereum (v1.15.11) - 以太坊核心数据结构
- etcd client (v3.5.10) - 领导者选举
- AWS SDK v2 - S3 操作
- kafka-go - Kafka 发布
