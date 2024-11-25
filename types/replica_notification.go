package types

import "github.com/ethereum/go-ethereum/common/hexutil"

type ReplicaStateChangeNotification struct {
	LatestBlockNumber *hexutil.Big   `json:"latestBlockNumber"`
	ReplicaStates     []ReplicaState `json:"replicaStates"`
}

// key := "<chainid>/replicaState/<endPoint>"
// value := json.Marshal(ReplicaState)
type ReplicaState struct {
	LatestBlockNumber uint64 `json:"latestBlockNumber"`
	EndPoint          string `json:"endPoint"`
}
