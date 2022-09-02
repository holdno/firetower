package protocol

import (
	"fmt"
	"io"
	"os"
	"time"
)

var (
	// LogLevel 日志打印的级别
	LogLevel = "INFO"
	// DefaultWriter 正常日志写入的地方
	DefaultWriter io.Writer = os.Stdout
	// DefaultErrorWriter 错误日志写入的地方
	DefaultErrorWriter io.Writer = os.Stderr
	// 日志颜色

	// Green 绿色
	Green = string([]byte{27, 91, 57, 55, 59, 52, 50, 109})
	// White 白色
	White = string([]byte{27, 91, 57, 48, 59, 52, 55, 109})
	// Yellow 黄色
	Yellow = string([]byte{27, 91, 57, 55, 59, 52, 51, 109})
	// Red 红色
	Red = string([]byte{27, 91, 57, 55, 59, 52, 49, 109})
	// Blue 蓝色
	Blue = string([]byte{27, 91, 57, 55, 59, 52, 52, 109})
	// Magenta 品红
	Magenta = string([]byte{27, 91, 57, 55, 59, 52, 53, 109})
	// Cyan 青色
	Cyan = string([]byte{27, 91, 57, 55, 59, 52, 54, 109})
	// Reset 重置日志颜色
	Reset = string([]byte{27, 91, 48, 109})
)

func fireLog(f *FireInfo, types, info string) {
	if types == "INFO" {
		if LogLevel != "INFO" {
			return
		}
		fmt.Fprintf(
			DefaultWriter,
			"[FireInfo] %s %s %s | LOGTIME %s | RUNTIME %s %v %s | MSGID %s | EVENT %s %s %s | TOPIC %s %s %s | DATA %s | LOG %s\n",
			Green, types, Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			Green, time.Since(f.Context.StartTime), Reset,
			f.Context.ID,
			Yellow, f.Message.Type, Reset,
			Cyan, f.Message.Topic, Reset,
			string(f.Message.Data),
			info)
	} else {
		fmt.Fprintf(
			DefaultErrorWriter,
			"[FireInfo] %s %s %s | LOGTIME %s | RUNTIME %s %v %s | MSGID %s | EVENT %s %s %s | TOPIC %s %s %s | DATA %s | LOG %s\n",
			Red, types, Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			Green, time.Since(f.Context.StartTime), Reset,
			f.Context.ID,
			Yellow, f.Message.Type, Reset,
			Cyan, f.Message.Topic, Reset,
			string(f.Message.Data),
			info)
	}

}
