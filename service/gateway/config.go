package gateway

import (
	"fmt"

	"github.com/pelletier/go-toml"
)

type FireTowerConfig struct {
	chanLens         int
	heartbeat        int
	heartbeatContent string
	topicServiceAddr string
}

var (
	ConfigTree *toml.Tree
)

func loadConfig() {
	var (
		err error
	)
	if ConfigTree, err = toml.LoadFile("./fireTower.toml"); err != nil {
		fmt.Println("config load failed:", err)
	}
}
