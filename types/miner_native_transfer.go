package types

type MinerNativeRransfer struct {
	ID     string  `json:"id"`
	ToAddr string  `json:"to_addr"`
	Value  float64 `json:"value"` //amount / 1e18
}
