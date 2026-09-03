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
	client          *clientv3.Client
	key             string
	nodeID          string
	ctx             context.Context
	cancel          context.CancelFunc
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

	// WriteLock fields
	writeLockKey     string
	writeLockTTL     int64
	writeLockLease   clientv3.Lease
	writeLockLeaseID clientv3.LeaseID
	writeLockCancel  context.CancelFunc
	writeLockMu      sync.Mutex
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

	// Set default writeLock TTL
	writeLockTTL := cfg.WriteLockTTL
	if writeLockTTL <= 0 {
		writeLockTTL = 10 // default 10 seconds
	} else if writeLockTTL > 30 {
		return nil, fmt.Errorf("writeLockTTL must be <= 30 seconds, got %d", writeLockTTL)
	}

	ctx, cancel := context.WithCancel(context.Background())
	lf := &LeaderFailover{
		client:       client,
		key:          cfg.Key,
		nodeID:       cfg.NodeID,
		ctx:          ctx,
		cancel:       cancel,
		state:        StateUnknown,
		gracePeriod:  cfg.GracePeriod,
		watcher:      clientv3.NewWatcher(client),
		writeLockKey: cfg.Key + "/writeLock",
		writeLockTTL: writeLockTTL,
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

	// Read persistent key
	resp, err := lf.client.Get(ctx, lf.key)
	if err != nil {
		return 0, err
	}

	// Read writeLock key
	wlResp, err := lf.client.Get(ctx, lf.writeLockKey)
	if err != nil {
		// WriteLock read failure doesn't block, just log
		log.Printf("[Leader Failover] Failed to read writeLock key: %v", err)
	}

	writeLockExists := wlResp != nil && len(wlResp.Kvs) > 0

	// If persistent key doesn't exist, enter initial election flow
	if len(resp.Kvs) == 0 {
		lf.etcdHealthy.Store(true)
		lf.applyNoLeader(resp.Header.Revision, false)
		if err := lf.tryToBecomeLeader(); err != nil {
			return resp.Header.Revision, err
		}
		// Re-read to confirm
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

	// Persistent key exists
	lf.etcdHealthy.Store(true)
	currentLeader := string(resp.Kvs[0].Value)

	// If persistent key points to self but writeLock doesn't exist, log it
	if currentLeader == lf.nodeID && !writeLockExists {
		log.Printf("[Leader Failover] Persistent key points to %s but writeLock missing during reconcile", lf.nodeID)
	}

	lf.applyLeaderValue(currentLeader, resp.Header.Revision)
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
		// Use WithPrefix to watch both leader key and writeLock key
		watchChan := lf.watcher.Watch(watchCtx, lf.key, clientv3.WithRev(revision), clientv3.WithPrefix())
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

					eventKey := string(event.Kv.Key)

					// Distinguish between leader key and writeLock key
					if eventKey == lf.key {
						lf.handleWatchEvent(event)
					} else if eventKey == lf.writeLockKey {
						lf.handleWriteLockEvent(event)
					}
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

// handleWriteLockEvent handles writeLock key changes
func (lf *LeaderFailover) handleWriteLockEvent(event *clientv3.Event) {
	currentLeader := lf.getCurrentLeader()

	switch event.Type {
	case clientv3.EventTypePut:
		// WriteLock updated, just log
		log.Printf("[Leader Failover] WriteLock updated: %s", string(event.Kv.Value))

	case clientv3.EventTypeDelete:
		// WriteLock disappeared, old leader has stopped writing
		log.Printf("[Leader Failover] WriteLock deleted, old leader has stopped writing")

		// Only attempt immediate promotion if persistent key points to self and not already leader
		if currentLeader == lf.nodeID && lf.State() == StateBackup {
			log.Printf("[Leader Failover] Persistent key points to %s and writeLock is gone, attempting immediate promotion", lf.nodeID)

			// Trigger immediate promotion (token already incremented in applyLeaderValue)
			lf.becomeLeaderAsync(lf.promotionToken.Load())
		}
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

	// Critical: if persistent key no longer points to self, immediately stop writeLock and step down
	if oldLeader == lf.nodeID && newLeader != lf.nodeID {
		log.Printf("[Leader Failover] Persistent key changed away from %s, stopping writeLock immediately", lf.nodeID)
		lf.promotionToken.Add(1)
		lf.transition(StateBackup) // This will call stopWriteLock if wasLeader
		return
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

	// Start writeLock BEFORE becoming leader (only if etcd client exists)
	if lf.client != nil {
		if err := lf.startWriteLock(); err != nil {
			log.Printf("[Leader Failover] Failed to start writeLock: %v, aborting promotion", err)
			go lf.retryPromotion(token)
			return
		}
	}

	// Only set state to Leader after writeLock is successfully created
	lf.state = StateLeader

	if lf.client != nil {
		log.Printf("[Leader Failover] Current node %s became LEADER with writeLock", lf.nodeID)
	} else {
		log.Printf("[Leader Failover] Current node %s became LEADER (no etcd client, writeLock skipped)", lf.nodeID)
	}
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
	if wasLeader {
		// Stop writeLock when losing leadership
		lf.stopWriteLock()

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
	// Also stop writeLock when transitioning to Unknown state (even if not from Leader)
	if state == StateUnknown {
		lf.stopWriteLock()
	}
}

func (lf *LeaderFailover) markUnknown(reason string) {
	// Flip health before waiting for an in-flight Kafka write to release the
	// mutex. Any newly arriving write observes this immediately and is denied.
	lf.etcdHealthy.Store(false)
	log.Printf("[Leader Failover] %s; pausing Kafka writes", reason)
	lf.transition(StateUnknown) // This will call stopWriteLock while holding mutex
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

// startWriteLock creates a lease and holds the writeLock key
func (lf *LeaderFailover) startWriteLock() error {
	lf.writeLockMu.Lock()
	defer lf.writeLockMu.Unlock()

	// Stop existing writeLock if running
	if lf.writeLockCancel != nil {
		lf.writeLockCancel()
		lf.writeLockCancel = nil
	}

	// Create lease
	lease := clientv3.NewLease(lf.client)
	grantResp, err := lease.Grant(lf.ctx, lf.writeLockTTL)
	if err != nil {
		return fmt.Errorf("failed to create writeLock lease: %w", err)
	}

	lf.writeLockLease = lease
	lf.writeLockLeaseID = grantResp.ID

	// Write writeLock key
	writeLockValue := fmt.Sprintf(`{"node_id":"%s","timestamp":%d}`,
		lf.nodeID, time.Now().Unix())

	ctx, cancel := context.WithTimeout(lf.ctx, 3*time.Second)
	defer cancel()

	_, err = lf.client.Put(ctx, lf.writeLockKey, writeLockValue,
		clientv3.WithLease(lf.writeLockLeaseID))
	if err != nil {
		lease.Revoke(context.Background(), lf.writeLockLeaseID)
		return fmt.Errorf("failed to write writeLock key: %w", err)
	}

	// Start KeepAlive
	kaCtx, kaCancel := context.WithCancel(lf.ctx)
	lf.writeLockCancel = kaCancel

	keepAliveCh, err := lease.KeepAlive(kaCtx, lf.writeLockLeaseID)
	if err != nil {
		kaCancel()
		lease.Revoke(context.Background(), lf.writeLockLeaseID)
		return fmt.Errorf("failed to start writeLock KeepAlive: %w", err)
	}

	// Start KeepAlive processor goroutine
	go lf.processWriteLockKeepAlive(keepAliveCh)

	log.Printf("[Leader Failover] Started writeLock for node %s (lease %d, TTL %ds)",
		lf.nodeID, lf.writeLockLeaseID, lf.writeLockTTL)

	return nil
}

// processWriteLockKeepAlive handles writeLock KeepAlive responses
func (lf *LeaderFailover) processWriteLockKeepAlive(keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse) {
	for {
		select {
		case <-lf.ctx.Done():
			log.Printf("[Leader Failover] WriteLock KeepAlive stopped (context done)")
			return
		case ka := <-keepAliveCh:
			if ka == nil {
				// KeepAlive channel closed - could be intentional shutdown or lease loss
				// Check if this is intentional shutdown
				if lf.ctx.Err() != nil {
					return
				}
				// Only mark unknown if we're still supposed to be leader
				if lf.State() == StateLeader {
					log.Printf("[Leader Failover] WriteLock lease lost for node %s, marking unknown", lf.nodeID)
					lf.markUnknown("writeLock lease lost")
				}
				return
			}
			// KeepAlive successful, continue
		}
	}
}

// stopWriteLock stops writeLock and deletes the key
func (lf *LeaderFailover) stopWriteLock() {
	lf.writeLockMu.Lock()
	defer lf.writeLockMu.Unlock()

	// Stop KeepAlive goroutine
	if lf.writeLockCancel != nil {
		lf.writeLockCancel()
		lf.writeLockCancel = nil
	}

	// Revoke lease (automatically deletes writeLock key)
	if lf.writeLockLease != nil && lf.writeLockLeaseID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := lf.writeLockLease.Revoke(ctx, lf.writeLockLeaseID)
		if err != nil {
			log.Printf("[Leader Failover] Failed to revoke writeLock lease: %v", err)
		} else {
			log.Printf("[Leader Failover] Revoked writeLock lease %d for node %s",
				lf.writeLockLeaseID, lf.nodeID)
		}

		lf.writeLockLeaseID = 0
	}
}

func (lf *LeaderFailover) Close() error {
	lf.closeOnce.Do(func() {
		lf.markUnknown("leader manager is closing") // This will call stopWriteLock via transition
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
