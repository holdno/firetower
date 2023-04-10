package config

import (
	"fmt"

	"github.com/pelletier/go-toml"
)

// FireTowerConfig 每个连接的配置信息
type FireTowerConfig struct {
	ChanLens    int
	Heartbeat   int
	ServiceMode string
	Bucket      BucketConfig
	Cluster     Cluster
}

type Cluster struct {
	RedisOption Redis
	NatsOption  Nats
}

type Redis struct {
	KeyPrefix string
	Addr      string
	Password  string
	DB        int
}

type Nats struct {
	Addr          string
	ServerName    string
	UserName      string
	Password      string
	SubjectPrefix string
}

type BucketConfig struct {
	Num              int
	CentralChanCount int64
	BuffChanCount    int64
	ConsumerNum      int
}

const (
	SingleMode         = "single"
	ClusterMode        = "cluster"
	DefaultServiceMode = SingleMode
)

var (
	// ConfigTree 保存配置
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
