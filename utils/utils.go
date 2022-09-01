package utils

import "github.com/holdno/snowFlakeByGo"

var (
	// IdWorker 全局唯一id生成器实例
	idWorker *snowFlakeByGo.Worker
)

func SetupIDWorker(clusterID int64) {
	idWorker, _ = snowFlakeByGo.NewWorker(clusterID)
}

func IDWorker() *snowFlakeByGo.Worker {
	return idWorker
}
