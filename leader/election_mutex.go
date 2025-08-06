package leader

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type LeaderElectionMutex struct {
	client       *clientv3.Client
	session      *concurrency.Session
	mutex        *concurrency.Mutex
	key          string
	nodeID       string
	ctx          context.Context
	cancel       context.CancelFunc
	IsLeaderNode bool
	LeaderMutex  sync.RWMutex
	callbacks    LeaderCallbacks
	gracePeriod  time.Duration
}

func NewLeaderElectionMutex(cfg Config) (*LeaderElectionMutex, error) {
	if cfg.GracePeriod == 0 {
		cfg.GracePeriod = 10 * time.Second
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LeaderElectionMutex{
		client:      client,
		key:         cfg.Key,
		nodeID:      cfg.NodeID,
		ctx:         ctx,
		cancel:      cancel,
		gracePeriod: cfg.GracePeriod,
	}, nil
}

func (le *LeaderElectionMutex) SetCallbacks(callbacks LeaderCallbacks) {
	le.callbacks = callbacks
}

func (le *LeaderElectionMutex) Start() error {
	session, err := concurrency.NewSession(le.client, concurrency.WithTTL(10))
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	le.session = session

	le.mutex = concurrency.NewMutex(session, le.key)

	go le.runLeaderLoop()

	return nil
}

func (le *LeaderElectionMutex) runLeaderLoop() {
	for {
		select {
		case <-le.ctx.Done():
			return
		default:
		}

		log.Printf("[Leader Election] Node %s attempting to acquire lock", le.nodeID)

		lockCtx, lockCancel := context.WithCancel(le.ctx)
		err := le.mutex.Lock(lockCtx)
		if err != nil {
			lockCancel()
			if err == context.Canceled {
				return
			}
			log.Printf("[Leader Election] Node %s failed to acquire lock: %v", le.nodeID, err)
			time.Sleep(5 * time.Second)
			continue
		}

		le.becomeLeader()

		select {
		case <-le.ctx.Done():
			lockCancel()
			le.loseLeadership()
			return
		case <-le.session.Done():
			log.Printf("[Leader Election] Node %s session expired, losing leadership", le.nodeID)
			lockCancel()
			le.loseLeadership()

			// recreate session
			session, err := concurrency.NewSession(le.client, concurrency.WithTTL(10))
			if err != nil {
				log.Printf("[Leader Election] Node %s failed to recreate session: %v", le.nodeID, err)
				time.Sleep(5 * time.Second)
				continue
			}
			le.session = session
			le.mutex = concurrency.NewMutex(session, le.key)
		}
	}
}

func (le *LeaderElectionMutex) becomeLeader() {
	// wait for the old leader to do clean up
	time.Sleep(le.gracePeriod)

	le.LeaderMutex.Lock()
	defer le.LeaderMutex.Unlock()

	le.IsLeaderNode = true
	log.Printf("[Leader Election] Node %s acquired lock, became LEADER", le.nodeID)

	if le.callbacks.OnBecomeLeader != nil {
		if err := le.callbacks.OnBecomeLeader(le.ctx); err != nil {
			log.Printf("[Leader Election] Node %s failed to execute OnBecomeLeader callback: %v", le.nodeID, err)
		}
	}
}

func (le *LeaderElectionMutex) loseLeadership() {
	le.LeaderMutex.Lock()

	if le.IsLeaderNode {
		log.Printf("[Leader Election] Node %s losing leadership, starting grace period (%v)", le.nodeID, le.gracePeriod)

		// 执行失去主节点的回调
		if le.callbacks.OnLoseLeader != nil {
			ctx, cancel := context.WithTimeout(context.Background(), le.gracePeriod)
			defer cancel()

			if err := le.callbacks.OnLoseLeader(ctx); err != nil {
				log.Printf("[Leader Election] Node %s failed to execute OnLoseLeader callback: %v", le.nodeID, err)
			}
		}
	}
	le.IsLeaderNode = false
	le.LeaderMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := le.mutex.Unlock(ctx); err != nil {
		log.Printf("[Leader Election] Node %s failed to unlock mutex: %v", le.nodeID, err)
	}
}

func (le *LeaderElectionMutex) IsLeader() bool {
	le.LeaderMutex.RLock()
	defer le.LeaderMutex.RUnlock()
	return le.IsLeaderNode
}

func (le *LeaderElectionMutex) IsBackup() bool {
	le.LeaderMutex.RLock()
	defer le.LeaderMutex.RUnlock()
	return !le.IsLeaderNode
}

func (le *LeaderElectionMutex) Stop() error {
	le.cancel()

	le.loseLeadership()

	if le.session != nil {
		return le.session.Close()
	}
	return nil
}

func (le *LeaderElectionMutex) Close() error {
	if err := le.Stop(); err != nil {
		return err
	}
	return le.client.Close()
}
