package gateway

import (
	"fmt"
	"github.com/pelletier/go-toml"
)

type BeaconTowerConfig struct {
	chanLens         int
	heartbeat        int
	heartbeatContent string
	topicServiceAddr string
}

var (
	BTConfig   *BeaconTowerConfig
	ConfigTree *toml.Tree
)

func loadConfig() {
	var (
		err error
	)
	if ConfigTree, err = toml.LoadFile("./beaconTower.toml"); err != nil {
		fmt.Println("config load failed:", err)
	}
}
