package types

// key := "replicaState/<chain_id>/available_nodes"
// value := uitls.EncodeToJsonGzip(AvailableReplicaStates)
type AvailableReplicaStates struct {
	LatestConsistencyBlockNumber uint64   `json:"latestConsistencyBlockNumber"` //副本的一致性高度
	EndPoints                    []string `json:"endPoints"`
}
