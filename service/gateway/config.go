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

func loadConfig(path string) {
	var (
		err error
	)
	if ConfigTree, err = toml.LoadFile(path); err != nil {
		fmt.Println("config load failed:", err)
	}
}
