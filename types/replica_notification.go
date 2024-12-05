package types

import "github.com/ethereum/go-ethereum/common/hexutil"

type ReplicaStateChangeNotification struct {
	LatestBlockNumber *hexutil.Big   `json:"latestBlockNumber"`
	ReplicaStates     []ReplicaState `json:"replicaStates"`
	NodeType          uint64         `json:"nodeType"` // 1 state, 2 archive
}

type ReplicaState struct {
	LatestBlockNumber *hexutil.Big `json:"latestBlockNumber"`
	StateType         uint64       `json:"stateType"` // 1 latest, 2 delay, 3 offline
	Address           string       `json:"address"`   //
}
