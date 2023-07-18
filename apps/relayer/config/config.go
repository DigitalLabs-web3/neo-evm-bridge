package config

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/nspcc-dev/neo-go/pkg/util"
)

type Config struct {
	MainSeeds         []string     `json:"mainSeeds"`
	SideSeeds         []string     `json:"sideSeeds"`
	VerifiedRootStart uint32       `json:"verifiedRootStart"`
	Start             uint32       `json:"start"`
	End               uint32       `json:"end"`
	BridgeContract    util.Uint160 `json:"bridgeContract"`
	Wallet            string       `json:"wallet"`
	Relayer           string       `json:"relayer"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	err = json.Unmarshal(b, cfg)
	if err != nil {
		return nil, err
	}
	err = cfg.check()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cfg *Config) check() error {
	if len(cfg.MainSeeds) == 0 {
		return errors.New("missing seeds")
	}
	if len(cfg.SideSeeds) == 0 {
		return errors.New("missing seeds")
	}
	if cfg.BridgeContract == (util.Uint160{}) {
		return errors.New("invalid manage contract address")
	}
	return nil
}
