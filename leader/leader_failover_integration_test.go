//go:build integration

package leader

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestPersistentManualSwitchAndEtcdReconnect(t *testing.T) {
	clientURL := unusedLocalURL(t)
	peerURL := unusedLocalURL(t)
	dataDir := filepath.Join(t.TempDir(), "etcd")
	etcd := startTestEtcd(t, dataDir, clientURL, peerURL, "new")

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{clientURL},
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	waitForEtcd(t, client)

	const key = "integration/writers/leader"
	gracePeriod := healthCheckInterval + healthCheckTimeout + time.Second
	nodeA := newIntegrationManager(t, clientURL, key, "node-a", gracePeriod)
	defer nodeA.Close()
	if err := nodeA.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, gracePeriod+3*time.Second, nodeA.IsLeader, "node-a initial election")

	nodeB := newIntegrationManager(t, clientURL, key, "node-b", gracePeriod)
	defer nodeB.Close()
	if err := nodeB.Start(); err != nil {
		t.Fatal(err)
	}
	if nodeB.IsLeader() {
		t.Fatal("node-b unexpectedly started as leader")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err = client.Put(ctx, key, "node-b")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return !nodeA.IsLeader() }, "node-a demotion")
	waitFor(t, gracePeriod+3*time.Second, nodeB.IsLeader, "node-b promotion")

	stopTestEtcd(t, etcd)
	waitFor(t, healthCheckInterval+healthCheckTimeout+3*time.Second, func() bool {
		return !nodeB.IsLeader()
	}, "node-b fail-close during etcd outage")

	etcd = startTestEtcd(t, dataDir, clientURL, peerURL, "existing")
	defer stopTestEtcd(t, etcd)
	waitFor(t, gracePeriod+10*time.Second, nodeB.IsLeader, "node-b recovery after etcd restart")

	if err := nodeA.Close(); err != nil {
		t.Fatal(err)
	}
	if err := nodeB.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	response, err := client.Get(ctx, key)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Kvs) != 1 || string(response.Kvs[0].Value) != "node-b" {
		t.Fatalf("persistent leader key after shutdown = %+v, want node-b", response.Kvs)
	}
}

func newIntegrationManager(t *testing.T, endpoint, key, nodeID string, grace time.Duration) *Manager {
	t.Helper()
	manager, err := NewManager(&ManagerConfig{
		EtcdEndpoints: []string{endpoint},
		ElectionKey:   key,
		NodeID:        nodeID,
		GracePeriod:   grace,
		OnBecomeLeader: func() error {
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func unusedLocalURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + address
}

func startTestEtcd(t *testing.T, dataDir, clientURL, peerURL, clusterState string) *exec.Cmd {
	t.Helper()
	command := exec.Command(
		"etcd",
		"--name", "pipeline-integration",
		"--data-dir", dataDir,
		"--listen-client-urls", clientURL,
		"--advertise-client-urls", clientURL,
		"--listen-peer-urls", peerURL,
		"--initial-advertise-peer-urls", peerURL,
		"--initial-cluster", fmt.Sprintf("pipeline-integration=%s", peerURL),
		"--initial-cluster-state", clusterState,
		"--log-level", "error",
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return command
}

func stopTestEtcd(t *testing.T, command *exec.Cmd) {
	t.Helper()
	if command == nil || command.Process == nil || command.ProcessState != nil {
		return
	}
	_ = command.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-done
	}
}

func waitForEtcd(t *testing.T, client *clientv3.Client) {
	t.Helper()
	waitFor(t, 10*time.Second, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, err := client.Get(ctx, "integration/health")
		return err == nil
	}, "local etcd startup")
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
