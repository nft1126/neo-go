package config

import (
	"time"

	"github.com/nspcc-dev/neo-go/pkg/core/storage"
	"github.com/nspcc-dev/neo-go/pkg/network/metrics"
	"github.com/nspcc-dev/neo-go/pkg/rpc"
)

// ApplicationConfiguration config specific to the node.
type ApplicationConfiguration struct {
	Address           string                  `yaml:"Address"`
	AttemptConnPeers  int                     `yaml:"AttemptConnPeers"`
	DBConfiguration   storage.DBConfiguration `yaml:"DBConfiguration"`
	DialTimeout       time.Duration           `yaml:"DialTimeout"`
	LogPath           string                  `yaml:"LogPath"`
	MaxPeers          int                     `yaml:"MaxPeers"`
	MinPeers          int                     `yaml:"MinPeers"`
	NodePort          uint16                  `yaml:"NodePort"`
	PingInterval      time.Duration           `yaml:"PingInterval"`
	PingTimeout       time.Duration           `yaml:"PingTimeout"`
	Pprof             metrics.Config          `yaml:"Pprof"`
	Prometheus        metrics.Config          `yaml:"Prometheus"`
	ProtoTickInterval time.Duration           `yaml:"ProtoTickInterval"`
	Relay             bool                    `yaml:"Relay"`
	RPC               rpc.Config              `yaml:"RPC"`
	UnlockWallet      Wallet                  `yaml:"UnlockWallet"`
	Oracle            OracleConfiguration     `yaml:"Oracle"`
}
