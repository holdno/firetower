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

func init() {
	var (
		err error
	)
	if ConfigTree, err = toml.LoadFile("/Users/wangboyan/development/golang/src/github.com/holdno/beacon/config/beaconTower.toml"); err != nil {
		fmt.Println("config load failed:", err)
	}

}
