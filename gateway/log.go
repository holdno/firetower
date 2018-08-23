package gateway

import (
	"fmt"
	"time"
)

const (
	prefix = "[firetower]"
)

// 打印日志信息
// 输出格式 [beacontower] 2018-08-09 17:49:30 INFO info
func (t *FireTower) LogInfo(info string) {
	fmt.Printf("%s %s %s %s\n", prefix, time.Now().Format("2006-01-02 15:03:04"), "INFO", info)
}

func (t *FireTower) LogError(err string) {
	fmt.Printf("%s %s %s %s\n", prefix, time.Now().Format("2006-01-02 15:03:04"), "ERROR", err)
}
