package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// WriterNodeInfo represents the writer node information stored in etcd
type WriterNodeInfo struct {
	NodeXBucket      string   `json:"node_x_bucket"`
	ChainTableBucket string   `json:"chain_table_bucket"`
	Region           string   `json:"region"`
	Brokers          []string `json:"brokers"`
	Topic            string   `json:"topic"`
}

// WriterRegistry manages writer node registration in etcd
type WriterRegistry struct {
	client   *clientv3.Client
	chainID  string
	nodeID   string
	nodeInfo WriterNodeInfo
	lease    clientv3.Lease
	leaseID  clientv3.LeaseID
	ttl      int64
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewWriterRegistry creates a new WriterRegistry instance
func NewWriterRegistry(client *clientv3.Client, chainID, nodeID string, nodeInfo WriterNodeInfo, ttl int64) *WriterRegistry {
	ctx, cancel := context.WithCancel(context.Background())

	return &WriterRegistry{
		client:   client,
		chainID:  chainID,
		nodeID:   nodeID,
		nodeInfo: nodeInfo,
		ttl:      ttl,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// RegisterNode registers the writer node in etcd with a lease
func (wr *WriterRegistry) RegisterNode() error {
	lease := clientv3.NewLease(wr.client)
	leaseResp, err := lease.Grant(wr.ctx, wr.ttl)
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}
	wr.leaseID = leaseResp.ID
	wr.lease = lease

	// Register node information
	nodeKey := wr.getNodeKey()
	nodeInfoBytes, err := json.Marshal(wr.nodeInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal node info: %w", err)
	}

	_, err = wr.client.Put(wr.ctx, nodeKey, string(nodeInfoBytes), clientv3.WithLease(wr.leaseID))
	if err != nil {
		// Revoke lease if put failed
		wr.lease.Revoke(context.Background(), wr.leaseID)
		return fmt.Errorf("failed to register node in etcd: %w", err)
	}

	// Keep lease alive
	keepAliveCh, err := wr.lease.KeepAlive(wr.ctx, wr.leaseID)
	if err != nil {
		return fmt.Errorf("failed to keep lease alive: %w", err)
	}

	// Start keep-alive processor
	go wr.processKeepAlive(keepAliveCh)

	log.Printf("[Writer Registry] Node %s registered successfully for chain %s with lease %d",
		wr.nodeID, wr.chainID, wr.leaseID)
	return nil
}

// UnregisterNode removes the writer node from etcd
func (wr *WriterRegistry) UnregisterNode() error {
	// Revoke lease, which will automatically delete the key
	_, err := wr.lease.Revoke(context.Background(), wr.leaseID)
	if err != nil {
		log.Printf("[Writer Registry] Failed to revoke lease: %v", err)
	}

	wr.cancel()

	log.Printf("[Writer Registry] Node %s unregistered from chain %s", wr.nodeID, wr.chainID)
	wr.lease.Close()
	return err
}

// processKeepAlive processes lease keep-alive responses
func (wr *WriterRegistry) processKeepAlive(keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse) {
	for {
		select {
		case <-wr.ctx.Done():
			log.Printf("[Writer Registry] Keep-alive processor stopped for node %s", wr.nodeID)
			return
		case ka := <-keepAliveCh:
			if ka == nil {
				log.Printf("[Writer Registry] Lease keep-alive channel closed for node %s, stopping registration", wr.nodeID)
				return
			}
			// Keep-alive successful, continue
		}
	}
}

func (wr *WriterRegistry) getNodeKey() string {
	return fmt.Sprintf("%s/writers/%s", wr.chainID, wr.nodeID)
}
