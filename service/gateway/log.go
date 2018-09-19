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
func logInfo(t *FireTower, info string) {
	TowerLogger(t, "INFO", info)
}

func logError(t *FireTower, err string) {
	TowerLogger(t, "ERROR", err)
}

func towerLog(t *FireTower, types, err string) {
	fmt.Fprintf(
		DefaultErrorWriter,
		"%s %s | TIME %s | CONNID %d | CLIENTID %s | USERID %s | INFO %s\n",
		"[FireTower]",
		types,
		time.Now().Format("2006-01-02 15:04:05"),
		t.connId,
		t.ClientId,
		t.UserId,
		err)
}

func fireLog(f *FireInfo, types, info string) {
	if types == "INFO" {
		if LogLevel != "INFO" {
			return
		}
		fmt.Fprintf(
			DefaultWriter,
			"%s %s | LOGTIME %s | RUNTIME %v | EVENT %s | TOPIC %s | DATA %s | LOG %s\n",
			"[FireInfo]",
			types,
			time.Now().Format("2006-01-02 15:04:05"),
			time.Since(f.Context.startTime),
			f.Message.Type,
			f.Message.Topic,
			string(f.Message.Data),
			info)
	} else {
		fmt.Fprintf(
			DefaultErrorWriter,
			"%s %s | LOGTIME %s | RUNTIME %v | EVENT %s | TOPIC %s | DATA %s | LOG %s\n",
			"[FireInfo]",
			types,
			time.Now().Format("2006-01-02 15:04:05"),
			time.Since(f.Context.startTime),
			f.Message.Type,
			f.Message.Topic,
			string(f.Message.Data),
			info)
	}

}
