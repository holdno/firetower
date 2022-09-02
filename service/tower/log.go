package tower

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/holdno/firetower/protocol"
)

var (
	// LogLevel Log Level 支持三种模式
	// INFO 打印所有日志信息
	// WARN 只打印警告及错误类型的日志信息
	// ERROR 只打印错误日志
	LogLevel = "INFO"
	// DefaultWriter 正常日志的默认写入方式
	DefaultWriter io.Writer = os.Stdout
	// DefaultErrorWriter 错误日志的默认写入方式
	DefaultErrorWriter io.Writer = os.Stderr
)

// 打印日志信息
// 输出格式 [firetower] 2018-08-09 17:49:30 INFO info
func logInfo(t *FireTower, info string) {
	TowerLogger(t, "INFO", info)
}

func towerLog(t *FireTower, types, err string) {
	fmt.Fprintf(
		DefaultErrorWriter,
		"[FireTower] %s %s %s | LOGTIME %s | RUNTIME %s %v %s | CONNID %d | CLIENTID %s | USERID %s | LOG %s\n",
		protocol.Green, types, protocol.Reset,
		time.Now().Format("2006-01-02 15:04:05"),
		protocol.Green, t.startTime.Format("2006-01-02 15:04:05"), protocol.Reset,
		t.connID,
		t.ClientID(),
		t.UserID(),
		err)
}
