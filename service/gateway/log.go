package gateway

import (
	"fmt"
	"github.com/holdno/firetower/socket"
	"io"
	"os"
	"time"
)

var (
	// Log Level 支持三种模式
	// INFO 打印所有日志信息
	// WARN 只打印警告及错误类型的日志信息
	// ERROR 只打印错误日志
	LogLevel                     = "INFO"
	DefaultWriter      io.Writer = os.Stdout
	DefaultErrorWriter io.Writer = os.Stderr
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
		"[FireTower] %s%s%s | LOGTIME %s | RUNTIME %s%v%s | MSGID %s | EVENT %s%s%s | TOPIC %s%s%s | DATA %s | LOG %s\n",
		socket.Green, types, socket.Reset,
		time.Now().Format("2006-01-02 15:04:05"),
		socket.Green, t.startTime.Format("2006-01-02 15:04:05"), socket.Reset,
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
			"[FireInfo] %s%s%s | LOGTIME %s | RUNTIME %s%v%s | MSGID %s | EVENT %s%s%s | TOPIC %s%s%s | DATA %s | LOG %s\n",
			socket.Green, types, socket.Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			socket.Green, time.Since(f.Context.startTime), socket.Reset,
			f.Context.id,
			socket.Yellow, f.Message.Type, socket.Reset,
			socket.Cyan, f.Message.Topic, socket.Reset,
			string(f.Message.Data),
			info)
	} else {
		fmt.Fprintf(
			DefaultErrorWriter,
			"%s %s | LOGTIME %s | RUNTIME %v | MSGID %s | EVENT %s | TOPIC %s | DATA %s | LOG %s\n",
			"[FireInfo]",
			socket.Red, types, socket.Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			socket.Green, time.Since(f.Context.startTime), socket.Reset,
			f.Context.id,
			socket.Yellow, f.Message.Type, socket.Reset,
			socket.Cyan, f.Message.Topic, socket.Reset,
			string(f.Message.Data),
			info)
	}

}
