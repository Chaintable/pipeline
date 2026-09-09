package failover

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdLock 是写节点主备切换的互斥锁，逻辑与 consistency-checker 的 EtcdLock 一致，
// 只是把「版本切换」换成了「写节点主备切换」：
//
//	versionKey -> <chain_id>/<version>/writers/leader 或 <chain_id>/writers/leader
//	             由运维写入，值是被指定为主节点的 nodeID
//	leaderKey  -> <versionKey>/writeLock
//	             由主节点持有，带 lease，主节点退出或宕机后自动消失
//	nodeID     -> 本节点标识，对应 checker 里的 version 字段
//
// 互斥性来自 leaderKey 上的事务：CreateRevision == 0 才能写入，因此同一时刻只有
// 一个节点能持有 writeLock。旧主节点宕机后 lease 到期，key 被删除，新主节点才能拿到锁，
// 这个空窗期天然替代了原实现里的 gracePeriod。
type EtcdLock struct {
	parent     context.Context
	leaderKey  string
	versionKey string
	nodeID     string
	client     *clientv3.Client
	ttl        int64
	lease      clientv3.LeaseID
	ctx        context.Context
	cancel     context.CancelFunc
	onLost     func()      // 锁丢失时的回调函数
	lost       atomic.Bool // 续约失败达到上限后置位
}

// NewLock 创建锁。parent 用于在进程关闭时中断阻塞中的 Acquire。
func NewLock(parent context.Context, leaderKey, versionKey, nodeID string, client *clientv3.Client, ttl int64, onLost func()) *EtcdLock {
	if parent == nil {
		parent = context.Background()
	}
	return &EtcdLock{
		parent:     parent,
		leaderKey:  leaderKey,
		versionKey: versionKey,
		nodeID:     nodeID,
		client:     client,
		ttl:        ttl,
		onLost:     onLost,
	}
}

// Acquire 尝试获取 writeLock。返回 false 表示本轮没拿到，调用方会在下一个检查周期重试。
//
// 注意：本函数可能阻塞在 watch 上等待旧锁释放，调用方必须在不持有 Manager 互斥锁的
// 情况下调用它，否则会阻塞区块提交路径。
func (l *EtcdLock) Acquire() bool {
	// 创建 context 用于取消操作。派生自 parent，进程关闭时可中断等待。
	l.ctx, l.cancel = context.WithCancel(l.parent)

	// 创建租约
	leaseResp, err := l.client.Grant(context.Background(), l.ttl)
	if err != nil {
		log.Printf("[Failover] Failed to create lease: %v", err)
		return false
	}
	log.Printf("[Failover] Lease created with ID: %d for lock key: %s", leaseResp.ID, l.leaderKey)
	l.lease = leaseResp.ID

	// 尝试获取锁
	for {
		// 使用事务来原子性地检查并设置锁
		// 条件: leaderKey 不存在 AND versionKey 的值等于 nodeID
		txn := l.client.Txn(context.Background())
		txn = txn.If(
			clientv3.Compare(clientv3.CreateRevision(l.leaderKey), "=", 0),
			clientv3.Compare(clientv3.Value(l.versionKey), "=", l.nodeID),
		).
			Then(clientv3.OpPut(l.leaderKey, l.nodeID, clientv3.WithLease(l.lease))).
			Else(clientv3.OpGet(l.leaderKey), clientv3.OpGet(l.versionKey))

		txnResp, err := txn.Commit()
		if err != nil {
			log.Printf("[Failover] Failed to commit transaction: %v", err)
			l.Release()
			return false
		}

		// 如果 If 条件为真，说明获取锁成功
		if txnResp.Succeeded {
			log.Printf("[Failover] Lock acquired successfully for key: %s", l.leaderKey)
			// 启动 WatchDog 续约
			go l.WatchDog()
			return true
		}

		// 获取锁失败，检查失败原因
		leaderResp := txnResp.Responses[0].GetResponseRange()
		versionResp := txnResp.Responses[1].GetResponseRange()

		// 检查主节点指派是否仍指向自己
		if len(versionResp.Kvs) > 0 {
			currentLeader := string(versionResp.Kvs[0].Value)
			if currentLeader != l.nodeID {
				log.Printf("[Failover] Leader mismatch: expected %s, got %s. Cannot acquire lock for key: %s",
					l.nodeID, currentLeader, l.leaderKey)
				l.Release()
				return false
			}
		} else {
			log.Printf("[Failover] Leader key %s does not exist. Cannot acquire lock for key: %s",
				l.versionKey, l.leaderKey)
			l.Release()
			return false
		}

		// 指派匹配但锁被占用，等待当前持有者释放锁
		log.Printf("[Failover] Lock is held by another process, waiting for release: %s", l.leaderKey)

		// 获取当前 key 的 revision，用于 watch
		if len(leaderResp.Kvs) == 0 {
			// key 已被删除，重试获取锁
			continue
		}

		// Watch key 的删除事件
		watchChan := l.client.Watch(l.ctx, l.leaderKey, clientv3.WithRev(leaderResp.Kvs[0].ModRevision))

		// 等待锁被释放
		select {
		case watchResp := <-watchChan:
			if watchResp.Err() != nil {
				log.Printf("[Failover] Watch error: %v, retrying lock acquisition", watchResp.Err())
				time.Sleep(time.Second)
				// watch 出错时重试获取锁，而不是放弃
				continue
			}

			// 检查是否有删除事件
			for _, event := range watchResp.Events {
				if event.Type == clientv3.EventTypeDelete {
					log.Printf("[Failover] Lock released, retrying acquisition: %s", l.leaderKey)
					break
				}
			}
			// 重试获取锁
			continue

		case <-l.ctx.Done():
			log.Printf("[Failover] Lock acquisition cancelled: %s", l.leaderKey)
			l.Release()
			return false
		}
	}
}

// Release 撤销租约并停止续约，writeLock key 随 lease 撤销自动删除。
//
// 这里给 Revoke 加了超时：Release 由持有 Manager 写锁的路径调用，不能被一次
// 卡住的 etcd 请求拖住区块提交。
func (l *EtcdLock) Release() {
	if l.lease != 0 {
		lease := l.lease
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := l.client.Revoke(ctx, lease)
		cancel()
		l.lease = 0
		if err != nil {
			// 撤销失败也要继续：lease 会在 TTL 到期后自动过期，key 随之删除。
			log.Printf("[Failover] Failed to revoke lease %d for key %s: %v", lease, l.leaderKey, err)
		} else {
			log.Printf("[Failover] Lock released for key: %s, lease: %d", l.leaderKey, lease)
		}
	}

	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
}

func (l *EtcdLock) WatchDog() {
	interval := time.Duration(l.ttl/4) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	failCount := 0
	maxFail := 3 // 允许连续失败 3 次，容忍单节点故障

	log.Printf("[Failover] WatchDog started for key %s (interval: %v, maxFail: %d)", l.leaderKey, interval, maxFail)

	for {
		select {
		case <-ticker.C:
			_, err := l.client.KeepAliveOnce(context.Background(), l.lease)
			if err != nil {
				failCount++
				log.Printf("[Failover] Failed to refresh lease (attempt %d/%d): %v", failCount, maxFail, err)
				if failCount >= maxFail {
					log.Printf("[Failover] Max retry reached, lock lost for key: %s", l.leaderKey)
					l.lease = 0 // 标记 lease 已失效
					// 先置位再回调：晋升流程可能还没把本锁挂到 Manager 上，
					// 它会在挂载前检查这个标记。
					l.lost.Store(true)
					if l.onLost != nil {
						l.onLost()
					}
					return
				}
			} else {
				if failCount > 0 {
					log.Printf("[Failover] Lease refresh recovered after %d failures", failCount)
				}
				failCount = 0 // 成功后重置计数
			}
		case <-l.ctx.Done():
			return
		}
	}
}

// Lost 返回本锁是否已经因续约失败而失效。
func (l *EtcdLock) Lost() bool { return l.lost.Load() }
