package main

import (
	"fmt"

	"github.com/OSMeteor/firetower/service/manager"
	"github.com/pelletier/go-toml"
)

// ConfigTree 配置信息
var ConfigTree *toml.Tree

func init() {
	var (
		err error
	)
	if ConfigTree, err = toml.LoadFile("./topicmanage.toml"); err != nil {
		fmt.Println("config load failed:", err)
	}
}

func main() {
	m := &manager.Manager{}
	go m.StartGrpcService(fmt.Sprintf(":%d", ConfigTree.Get("grpc.port").(int64)))
	m.StartSocketService(fmt.Sprintf("0.0.0.0:%d", ConfigTree.Get("socket.port").(int64)))
}
