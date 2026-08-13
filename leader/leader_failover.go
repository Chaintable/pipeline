package leader

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// State describes the locally known writer role. Unknown is deliberately
// different from Backup: while etcd is unavailable we must not write Kafka.
type State uint8

const (
	StateUnknown State = iota
	StateBackup
	StateLeader
)

func (s State) String() string {
	switch s {
	case StateLeader:
		return "leader"
	case StateBackup:
		return "backup"
	default:
		return "unknown"
	}
}

const (
	healthCheckInterval = 5 * time.Second
	healthCheckTimeout  = 3 * time.Second
	watchRetryMin       = 250 * time.Millisecond
	watchRetryMax       = 10 * time.Second
)

type LeaderFailover struct {
	client *clientv3.Client
	key    string
	nodeID string
	ctx    context.Context
	cancel context.CancelFunc
	// IsLeaderNode is retained for source compatibility. Access it through
	// IsLeader or IsLeaderLocked; it is always updated while LeaderMutex is held.
	IsLeaderNode    bool
	LeaderMutex     sync.RWMutex
	state           State
	callbacks       LeaderCallbacks
	gracePeriod     time.Duration
	watcher         clientv3.Watcher
	roleUpdateMu    sync.Mutex
	leaderValueMu   sync.Mutex
	currentLeader   string
	currentRevision int64
	etcdHealthy     atomic.Bool
	promotionToken  atomic.Int64
	closeOnce       sync.Once
	closeErr        error
}

func NewLeaderFailover(cfg Config) (*LeaderFailover, error) {
	if cfg.GracePeriod <= healthCheckInterval+healthCheckTimeout {
		return nil, fmt.Errorf(
			"grace period %s must be greater than %s so an unreachable old leader closes its Kafka gate first",
			cfg.GracePeriod, healthCheckInterval+healthCheckTimeout,
		)
	}
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	lf := &LeaderFailover{
		client:      client,
		key:         cfg.Key,
		nodeID:      cfg.NodeID,
		ctx:         ctx,
		cancel:      cancel,
		state:       StateUnknown,
		gracePeriod: cfg.GracePeriod,
		watcher:     clientv3.NewWatcher(client),
	}
	return lf, nil
}

func (lf *LeaderFailover) SetCallbacks(callbacks LeaderCallbacks) { lf.callbacks = callbacks }

// Start performs one consistent read/election and then starts the watch loop.
// The watch loop owns all subsequent etcd reconnects; no lease or TTL is used
// for the persistent leader key.
func (lf *LeaderFailover) Start() error {
	revision, err := lf.reconcile()
	if err != nil {
		return fmt.Errorf("[Leader Failover] initial etcd sync failed: %w", err)
	}
	go lf.watchLeaderChangesFromRevision(revision + 1)
	return nil
}

// reconcile closes the Get-to-Watch gap by returning the revision of the
// authoritative read. If the key is absent, it attempts the initial election
// and reads the key again so the resulting role is applied synchronously.
func (lf *LeaderFailover) reconcile() (int64, error) {
	ctx, cancel := context.WithTimeout(lf.ctx, 5*time.Second)
	defer cancel()

	resp, err := lf.client.Get(ctx, lf.key)
	if err != nil {
		return 0, err
	}
	if len(resp.Kvs) == 0 {
		lf.etcdHealthy.Store(true)
		lf.applyNoLeader(resp.Header.Revision, false)
		if err := lf.tryToBecomeLeader(); err != nil {
			return resp.Header.Revision, err
		}
		resp, err = lf.client.Get(ctx, lf.key)
		if err != nil {
			return 0, err
		}
	}

	if len(resp.Kvs) == 0 {
		lf.etcdHealthy.Store(true)
		lf.applyNoLeader(resp.Header.Revision, false)
		return resp.Header.Revision, nil
	}
	lf.etcdHealthy.Store(true)
	lf.applyLeaderValue(string(resp.Kvs[0].Value), resp.Header.Revision)
	return resp.Header.Revision, nil
}

func (lf *LeaderFailover) tryToBecomeLeader() error {
	ctx, cancel := context.WithTimeout(lf.ctx, 5*time.Second)
	defer cancel()

	if lf.getCurrentLeader() != "" {
		return nil
	}
	txnResp, err := lf.client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(lf.key), "=", 0)).
		Then(clientv3.OpPut(lf.key, lf.nodeID)).
		Else(clientv3.OpGet(lf.key)).Commit()
	if err != nil {
		return fmt.Errorf("failed to set leader: %w", err)
	}
	if txnResp.Succeeded {
		log.Printf("[Leader Failover] Successfully set persistent leader key to %s", lf.nodeID)
		lf.applyLeaderValue(lf.nodeID, txnResp.Header.Revision)
		return nil
	}
	if len(txnResp.Responses) > 0 {
		if rangeResp := txnResp.Responses[0].GetResponseRange(); rangeResp != nil && len(rangeResp.Kvs) > 0 {
			currentLeader := string(rangeResp.Kvs[0].Value)
			lf.applyLeaderValue(currentLeader, txnResp.Header.Revision)
			log.Printf("[Leader Failover] Another node (%s) is already leader", currentLeader)
		}
	}
	return nil
}

func (lf *LeaderFailover) watchLeaderChangesFromRevision(revision int64) {
	log.Printf("[Leader Failover] Starting resilient watch from revision %d", revision)
	backoff := watchRetryMin
	for {
		if lf.ctx.Err() != nil {
			return
		}
		watchCtx, watchCancel := context.WithCancel(lf.ctx)
		watchChan := lf.watcher.Watch(watchCtx, lf.key, clientv3.WithRev(revision))
		healthCheck := time.NewTicker(healthCheckInterval)
		watchFailed := false
		for !watchFailed {
			select {
			case <-lf.ctx.Done():
				healthCheck.Stop()
				watchCancel()
				return
			case resp, ok := <-watchChan:
				if !ok {
					lf.markUnknown("etcd watch channel closed")
					watchFailed = true
					break
				}
				if err := resp.Err(); err != nil {
					lf.markUnknown(fmt.Sprintf("etcd watch failed: %v", err))
					watchFailed = true
					break
				}
				if resp.Header.Revision >= revision {
					revision = resp.Header.Revision + 1
				}
				for _, event := range resp.Events {
					if event.Kv != nil && event.Kv.ModRevision+1 > revision {
						revision = event.Kv.ModRevision + 1
					}
					lf.handleWatchEvent(event)
				}
			case <-healthCheck.C:
				healthRevision, err := lf.healthCheck()
				if err != nil {
					lf.markUnknown(fmt.Sprintf("etcd health check failed: %v", err))
					watchFailed = true
				} else if healthRevision >= revision {
					revision = healthRevision + 1
				}
			}
		}
		healthCheck.Stop()
		watchCancel()
		if lf.ctx.Err() != nil {
			return
		}

		// Re-read the key after every watch failure. WithRev(revision) below
		// covers events that happened between this read and the next watch.
		for {
			nextRevision, err := lf.reconcile()
			if err == nil {
				revision = nextRevision + 1
				backoff = watchRetryMin
				break
			}
			lf.markUnknown(fmt.Sprintf("etcd reconnect failed: %v", err))
			select {
			case <-lf.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > watchRetryMax {
				backoff = watchRetryMax
			}
		}
	}
}

func (lf *LeaderFailover) healthCheck() (int64, error) {
	ctx, cancel := context.WithTimeout(lf.ctx, healthCheckTimeout)
	defer cancel()
	resp, err := lf.client.Get(ctx, lf.key)
	if err != nil {
		return 0, err
	}
	lf.etcdHealthy.Store(true)
	if len(resp.Kvs) == 0 {
		lf.applyNoLeader(resp.Header.Revision, true)
	} else {
		lf.applyLeaderValue(string(resp.Kvs[0].Value), resp.Header.Revision)
	}
	return resp.Header.Revision, nil
}

func (lf *LeaderFailover) handleWatchEvent(event *clientv3.Event) {
	switch event.Type {
	case clientv3.EventTypePut:
		lf.applyLeaderValue(string(event.Kv.Value), event.Kv.ModRevision)
	case clientv3.EventTypeDelete:
		lf.applyNoLeader(event.Kv.ModRevision, true)
	}
}

func (lf *LeaderFailover) applyLeaderValue(newLeader string, revision int64) {
	lf.roleUpdateMu.Lock()
	defer lf.roleUpdateMu.Unlock()
	oldLeader := lf.getCurrentLeader()
	if !lf.updateLeaderValue(newLeader, revision) {
		return
	}
	if oldLeader == newLeader && lf.State() != StateUnknown {
		return
	}
	if oldLeader != newLeader {
		log.Printf("[Leader Failover] Leader changed from %s to %s, current node %s", oldLeader, newLeader, lf.nodeID)
	}
	if newLeader == lf.nodeID {
		// A healthy read after an Unknown period starts a fresh grace window.
		if lf.State() == StateUnknown {
			lf.transition(StateBackup)
		}
		lf.becomeLeaderAsync(lf.promotionToken.Add(1))
	} else {
		lf.promotionToken.Add(1)
		lf.transition(StateBackup)
	}
}

func (lf *LeaderFailover) applyNoLeader(revision int64, scheduleElection bool) {
	lf.roleUpdateMu.Lock()
	oldLeader := lf.getCurrentLeader()
	if !lf.updateLeaderValue("", revision) {
		lf.roleUpdateMu.Unlock()
		return
	}
	lf.promotionToken.Add(1)
	lf.transition(StateBackup)
	lf.roleUpdateMu.Unlock()
	if !scheduleElection {
		return
	}
	go func() {
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
		if lf.ctx.Err() != nil || lf.getCurrentLeader() != "" {
			return
		}
		log.Printf("[Leader Failover] Key deleted (old leader was %s), attempting election", oldLeader)
		if err := lf.tryToBecomeLeader(); err != nil {
			log.Printf("[Leader Failover] Failed to become leader after key deletion: %v", err)
		}
	}()
}

func (lf *LeaderFailover) becomeLeaderAsync(token int64) { go lf.becomeLeader(token) }

func (lf *LeaderFailover) becomeLeader(token int64) {
	log.Printf("[Leader Failover] Current node %s waiting grace period (%v) before becoming leader", lf.nodeID, lf.gracePeriod)
	timer := time.NewTimer(lf.gracePeriod)
	defer timer.Stop()
	select {
	case <-lf.ctx.Done():
		return
	case <-timer.C:
	}

	lf.LeaderMutex.Lock()
	defer lf.LeaderMutex.Unlock()
	// The key may have changed while waiting. Never promote on a stale event.
	if token != lf.promotionToken.Load() || lf.getCurrentLeader() != lf.nodeID || !lf.etcdHealthy.Load() || lf.state == StateUnknown || lf.state == StateLeader {
		return
	}
	if lf.callbacks.OnBecomeLeader != nil {
		if err := lf.callbacks.OnBecomeLeader(lf.ctx); err != nil {
			log.Printf("[Leader Failover] Current node %s failed OnBecomeLeader: %v", lf.nodeID, err)
			go lf.retryPromotion(token)
			return
		}
	}
	// Check again because the watch path updates these before waiting for this
	// mutex. A manual switch during checkpoint initialization must win.
	if lf.getCurrentLeader() != lf.nodeID || !lf.etcdHealthy.Load() {
		return
	}
	lf.state = StateLeader
	lf.IsLeaderNode = true
	log.Printf("[Leader Failover] Current node %s became LEADER", lf.nodeID)
}

func (lf *LeaderFailover) retryPromotion(token int64) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-lf.ctx.Done():
		return
	case <-timer.C:
		lf.becomeLeader(token)
	}
}

func (lf *LeaderFailover) transition(state State) {
	lf.LeaderMutex.Lock()
	defer lf.LeaderMutex.Unlock()
	if state == StateLeader || lf.state == state {
		return
	}
	wasLeader := lf.state == StateLeader
	lf.state = state
	lf.IsLeaderNode = state == StateLeader
	if wasLeader {
		// Leadership loss is intentionally lightweight. The mutex gates Kafka
		// writes; S3 uploads are allowed to finish and are not cancelled.
		ctx, cancel := context.WithTimeout(lf.ctx, lf.gracePeriod)
		defer cancel()
		if lf.callbacks.OnLoseLeader != nil {
			if err := lf.callbacks.OnLoseLeader(ctx); err != nil {
				log.Printf("[Leader Failover] Current node %s failed OnLoseLeader: %v", lf.nodeID, err)
			}
		}
		log.Printf("[Leader Failover] Current node %s is no longer leader (state=%s)", lf.nodeID, state)
	}
}

func (lf *LeaderFailover) markUnknown(reason string) {
	// Flip health before waiting for an in-flight Kafka write to release the
	// mutex. Any newly arriving write observes this immediately and is denied.
	lf.etcdHealthy.Store(false)
	log.Printf("[Leader Failover] %s; pausing Kafka writes", reason)
	lf.transition(StateUnknown)
}

func (lf *LeaderFailover) IsLeader() bool {
	lf.LeaderMutex.RLock()
	defer lf.LeaderMutex.RUnlock()
	return lf.IsLeaderLocked()
}

func (lf *LeaderFailover) IsLeaderLocked() bool {
	return lf.state == StateLeader && lf.etcdHealthy.Load() && lf.getCurrentLeader() == lf.nodeID
}

func (lf *LeaderFailover) State() State {
	lf.LeaderMutex.RLock()
	defer lf.LeaderMutex.RUnlock()
	return lf.state
}

func (lf *LeaderFailover) IsBackup() bool { return lf.State() == StateBackup }

func (lf *LeaderFailover) getCurrentLeader() string {
	lf.leaderValueMu.Lock()
	defer lf.leaderValueMu.Unlock()
	return lf.currentLeader
}

func (lf *LeaderFailover) updateLeaderValue(leader string, revision int64) bool {
	lf.leaderValueMu.Lock()
	defer lf.leaderValueMu.Unlock()
	if revision < lf.currentRevision {
		return false
	}
	lf.currentLeader = leader
	lf.currentRevision = revision
	return true
}

func (lf *LeaderFailover) Stop() error { lf.cancel(); return nil }

func (lf *LeaderFailover) Close() error {
	lf.closeOnce.Do(func() {
		lf.markUnknown("leader manager is closing")
		lf.cancel()
		if lf.watcher != nil {
			lf.closeErr = lf.watcher.Close()
		}
		if err := lf.client.Close(); lf.closeErr == nil {
			lf.closeErr = err
		}
	})
	return lf.closeErr
}
