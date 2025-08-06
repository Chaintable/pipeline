# Leader Election for Pipeline

该模块实现了基于etcd分布式锁（Mutex）的主备自动切换功能，相比使用Election API更加简单直观。

## 功能特性

1. **自动主备切换**：通过etcd分布式锁实现节点间的自动主备切换
2. **安全切换机制**：当主节点丢失锁时，会有grace period确保旧主节点停止写操作后，新主节点才开始工作
3. **手动模式支持**：保留了原有的isBackup参数，当etcd不可用时可以手动设置主备状态

## 配置说明

在blockchain-gethx的配置中添加以下参数：

```json
{
  "live": {
    "pipeline": {
      "region": "xxx",
      "node_x_bucket": "xxx",
      "chain_table_bucket": "xxx",
      "brokers": ["kafka1:9092", "kafka2:9092"],
      "topic": "xxx",
      "s3_temp_dir": "/tmp/s3",
      "is_backup": false,
      "enable_prestate_tracer": false,
      
      // 新增的选举配置
      "enable_election": true,
      "etcd_endpoints": ["etcd1:2379", "etcd2:2379", "etcd3:2379"],
      "election_key": "/pipeline/leader",
      "node_id": "node-1"
    }
  }
}
```

### 参数说明

- `enable_election`: 是否启用etcd选举功能
- `etcd_endpoints`: etcd集群地址列表
- `election_key`: 选举使用的key前缀，不同的集群应使用不同的key
- `node_id`: 当前节点的唯一标识
- `is_backup`: 手动模式下的主备状态，当etcd不可用时生效

## 工作原理

1. **启动时**：
   - 如果`enable_election=false`或未配置etcd，使用传统的`is_backup`参数
   - 如果`enable_election=true`且配置了etcd，自动参与选举

2. **主节点行为**：
   - 获得分布式锁的节点成为主节点
   - 主节点会覆盖写S3数据
   - 主节点会推送消息到Kafka

3. **备份节点行为**：
   - 未获得锁的节点为备份节点
   - 备份节点上传S3但不覆盖已有文件
   - 备份节点不推送Kafka消息

4. **主备切换**：
   - 当主节点失去锁时（网络故障、手动停止等），会有5秒的grace period
   - Grace period期间，旧主节点停止Kafka推送
   - 新主节点等待grace period结束后才开始工作
   - 这确保了不会同时有两个节点向Kafka写入

## 故障处理

1. **etcd不可用**：如果etcd集群故障，可以通过设置`is_backup`参数手动控制主备状态
2. **网络分区**：grace period机制确保即使出现网络分区，也不会有两个主节点同时工作
3. **节点重启**：节点重启后会自动参与选举，如果成为主节点会等待安全时间后开始工作

## 监控

通过日志可以监控主备切换状态：
- "acquired lock, became leader" - 节点获得锁成为主节点
- "losing leadership" - 节点失去主节点身份
- "Transitioning from leader to backup" - 主节点转为备份
- "Transitioning from backup to leader" - 备份转为主节点

## 实现原理

### 使用分布式锁而非Election API的优势

1. **更简单的实现**：
   - 分布式锁：Lock() → 持有锁 → Unlock()/Session过期
   - Election API：Campaign() → Observe() → Resign()

2. **更清晰的语义**：
   - 有锁 = 主节点
   - 无锁 = 备份节点
   - 不需要额外的状态监听

3. **核心代码**：
```go
// 竞争锁
err := mutex.Lock(ctx)
if err == nil {
    // 成为主节点
    becomeLeader()
    
    // 监听session状态
    <-session.Done()
    
    // 失去锁，变为备份
    loseLeadership()
}
```

### 安全机制

1. **Session TTL**：10秒心跳，确保节点故障时自动释放锁
2. **Grace Period**：5秒宽限期，避免双主问题
3. **原子操作**：使用atomic.Bool确保状态一致性