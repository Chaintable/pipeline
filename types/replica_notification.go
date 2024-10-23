package types

import "github.com/ethereum/go-ethereum/common/hexutil"

type ReplicaStateChangeNotification struct {
	LatestBlockNumber *hexutil.Big   `json:"latestBlockNumber"`
	ReplicaStates     []ReplicaState `json:"replicaStates"`
}

type ReplicaState struct {
	LatestBlockNumber *hexutil.Big `json:"latestBlockNumber"`
	StateType         uint64       `json:"stateType"` // 1 latest, 2 delay, 3 offline
	Meta              string       `json:"meta"`      // 副本元数据，ip等，和网关模块进一步约定
}
