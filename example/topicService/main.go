package main

import (
	"fmt"
	"github.com/holdno/firetower/service/manager"
	"github.com/pelletier/go-toml"
)

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
	go manager.StartGrpcService(fmt.Sprintf(":%d", ConfigTree.Get("grpc.port").(int64)))
	manager.StartSocketService(fmt.Sprintf("0.0.0.0:%d", ConfigTree.Get("socket.port").(int64)))
}
