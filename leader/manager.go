package leader

import (
	"context"
	"log"
	"time"
)

// GlobalManager is a global reference to the leader manager
var GlobalManager *Manager

type Manager struct {
	Election       *LeaderElectionMutex
	manualMode     bool
	isManualBackup bool
}

type ManagerConfig struct {
	EtcdEndpoints  []string
	ElectionKey    string
	NodeID         string
	IsBackup       *bool // nil = use etcd, non-nil = manual mode with specified value
	OnBecomeLeader func() error
	OnLoseLeader   func() error
}

func NewManager(cfg ManagerConfig) (*Manager, error) {
	m := &Manager{}

	if cfg.IsBackup != nil {
		// Manual mode - set fixed backup state
		m.manualMode = true
		m.isManualBackup = *cfg.IsBackup
		log.Printf("[Manager] Created in manual mode, isBackup=%v", *cfg.IsBackup)
	} else {
		electionCfg := Config{
			Endpoints:   cfg.EtcdEndpoints,
			Key:         cfg.ElectionKey,
			NodeID:      cfg.NodeID,
			GracePeriod: 10 * time.Second,
		}

		election, err := NewLeaderElectionMutex(electionCfg)
		if err != nil {
			return nil, err
		}

		election.SetCallbacks(LeaderCallbacks{
			OnBecomeLeader: func(ctx context.Context) error {
				log.Printf("[Manager] Becoming leader, setting backup to false")
				if cfg.OnBecomeLeader != nil {
					return cfg.OnBecomeLeader()
				}
				return nil
			},
			OnLoseLeader: func(ctx context.Context) error {
				log.Printf("[Manager] Losing leader, setting backup to true")
				if cfg.OnLoseLeader != nil {
					return cfg.OnLoseLeader()
				}
				return nil
			},
		})

		m.Election = election
	}

	return m, nil
}

func (m *Manager) Start() error {
	if m.manualMode {
		// Manual mode - nothing to start
		return nil
	}
	if m.Election != nil {
		return m.Election.Start()
	}
	return nil
}

func (m *Manager) IsLeader() bool {
	if m.manualMode {
		return !m.isManualBackup
	}
	return m.Election.IsLeader()
}

func (m *Manager) IsBackup() bool {
	if m.manualMode {
		return m.isManualBackup
	}
	return m.Election.IsBackup()
}

func (m *Manager) Lock() {
	m.Election.LeaderMutex.Lock()
}

func (m *Manager) Unlock() {
	m.Election.LeaderMutex.Unlock()
}

func (m *Manager) RLock() {
	m.Election.LeaderMutex.RLock()
}

func (m *Manager) RUnlock() {
	m.Election.LeaderMutex.RUnlock()
}

func (m *Manager) Stop() error {
	if m.Election != nil {
		return m.Election.Stop()
	}
	return nil
}

func (m *Manager) Close() error {
	if m.Election != nil {
		return m.Election.Close()
	}
	return nil
}
