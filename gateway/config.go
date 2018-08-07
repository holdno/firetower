package logic

import (
	"fmt"
	"github.com/pelletier/go-toml"
)

type BeaconTowerConfig struct {
	chanLens         int
	heartbeat        int
	heartbeatContent string
}

var (
	BTConfig *BeaconTowerConfig
)

func init() {
	var (
		tree *toml.Tree
		err  error
	)
	if tree, err = toml.LoadFile("./config/beaconTower.toml"); err != nil {
		fmt.Println("config load failed:", err)
	} else {
		BTConfig = new(BeaconTowerConfig)
		tree.Unmarshal(BTConfig)
	}

}
