package leader

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func newStateTestFailover(gracePeriod time.Duration) *LeaderFailover {
	ctx, cancel := context.WithCancel(context.Background())
	lf := &LeaderFailover{
		nodeID:      "node-a",
		ctx:         ctx,
		cancel:      cancel,
		state:       StateBackup,
		gracePeriod: gracePeriod,
	}
	lf.etcdHealthy.Store(true)
	return lf
}

func TestStaleLeaderEventCannotPromote(t *testing.T) {
	lf := newStateTestFailover(10 * time.Millisecond)
	defer lf.cancel()

	lf.applyLeaderValue("node-a", 1)
	lf.applyLeaderValue("node-b", 2)
	time.Sleep(30 * time.Millisecond)
	if lf.IsLeader() {
		t.Fatal("node was promoted after the leader key changed during grace period")
	}
}

func TestUnknownStateImmediatelyClosesKafkaGate(t *testing.T) {
	lf := newStateTestFailover(0)
	lf.updateLeaderValue("node-a", 1)
	lf.state = StateLeader
	lf.IsLeaderNode = true

	lf.etcdHealthy.Store(false)
	if lf.IsLeaderLocked() {
		t.Fatal("Kafka gate stayed open after etcd health was lost")
	}
}

func TestLeadershipCallbacksRunOnce(t *testing.T) {
	lf := newStateTestFailover(0)
	defer lf.cancel()
	var became atomic.Int32
	var lost atomic.Int32
	lf.SetCallbacks(LeaderCallbacks{
		OnBecomeLeader: func(context.Context) error { became.Add(1); return nil },
		OnLoseLeader:   func(context.Context) error { lost.Add(1); return nil },
	})

	lf.applyLeaderValue("node-a", 1)
	deadline := time.Now().Add(time.Second)
	for !lf.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	lf.applyLeaderValue("node-a", 1)
	time.Sleep(10 * time.Millisecond)
	lf.applyLeaderValue("node-b", 2)

	if got := became.Load(); got != 1 {
		t.Fatalf("OnBecomeLeader calls = %d, want 1", got)
	}
	if got := lost.Load(); got != 1 {
		t.Fatalf("OnLoseLeader calls = %d, want 1", got)
	}
}

func TestOlderRevisionCannotOverwriteLeader(t *testing.T) {
	lf := newStateTestFailover(0)
	lf.applyLeaderValue("node-b", 20)
	lf.applyLeaderValue("node-a", 19)
	if got := lf.getCurrentLeader(); got != "node-b" {
		t.Fatalf("current leader = %q, want node-b", got)
	}
}

func TestPromotionWaitsForSuccessfulCallback(t *testing.T) {
	lf := newStateTestFailover(0)
	defer lf.cancel()
	var calls atomic.Int32
	lf.SetCallbacks(LeaderCallbacks{
		OnBecomeLeader: func(context.Context) error {
			if calls.Add(1) == 1 {
				return context.DeadlineExceeded
			}
			return nil
		},
	})

	lf.applyLeaderValue("node-a", 1)
	time.Sleep(20 * time.Millisecond)
	if lf.IsLeader() {
		t.Fatal("node became leader after checkpoint initialization failed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !lf.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !lf.IsLeader() {
		t.Fatal("node did not retry promotion after checkpoint initialization recovered")
	}
}

func TestGracePeriodMustCoverWatchFailureDetection(t *testing.T) {
	_, err := NewLeaderFailover(Config{
		Endpoints:   []string{"http://127.0.0.1:2379"},
		Key:         "1/writers/leader",
		NodeID:      "node-a",
		GracePeriod: healthCheckInterval + healthCheckTimeout,
	})
	if err == nil {
		t.Fatal("NewLeaderFailover accepted a grace period shorter than failure detection")
	}
}
