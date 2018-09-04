package gateway

import (
	"fmt"
	"time"
)

const (
	prefix = "[firetower]"
)

// 打印日志信息
// 输出格式 [firetower] 2018-08-09 17:49:30 INFO info
func (t *FireTower) LogInfo(info string) {
	if t.logger != nil {
		t.logger("INFO", info)
	} else {
		fmt.Printf("%s %s %s %s\n", prefix, time.Now().Format("2006-01-02 15:04:05"), "INFO", info)
	}
}

func (t *FireTower) LogError(err string) {
	if t.logger != nil {
		t.logger("ERROR", err)
	} else {
		fmt.Printf("%s %s %s %s\n", prefix, time.Now().Format("2006-01-02 15:04:05"), "ERROR", err)
	}
}
