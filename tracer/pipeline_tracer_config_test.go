package tracer

import (
	"encoding/json"
	"testing"
)

func TestEnableOrbitGenesisTransactions(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want bool
	}{
		{name: "default", cfg: `{"is_backup":true}`, want: false},
		{name: "enabled", cfg: `{"is_backup":true,"enable_orbit_genesis_transactions":true}`, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracer, err := NewPipelineTracer(json.RawMessage(test.cfg))
			if err != nil {
				t.Fatalf("NewPipelineTracer() error = %v", err)
			}
			if got := tracer.EnableOrbitGenesisTransactions(); got != test.want {
				t.Fatalf("EnableOrbitGenesisTransactions() = %t, want %t", got, test.want)
			}
		})
	}
}
