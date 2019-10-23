package socket

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
	// SendLogger 消息相关的日志处理方法
	SendLogger func(s *SendMessage, types, info string)
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

// Recycling 回收SendMessage对象
func (s *SendMessage) Recycling() {
	sendPool.Put(s)
}

// Panic 记录一个Panic错误日志并回收SendMessage对象
func (s *SendMessage) Panic(info string) {
	SendLogger(s, "Panic", info)
	s.Recycling()
}

// Info 记录一个INFO级别的日志
func (s *SendMessage) Info(info string) {
	SendLogger(s, "INFO", info)
}

// Error 记录一个ERROR级别的日志
func (s *SendMessage) Error(info string) {
	SendLogger(s, "ERROR", info)
}

func sendLog(s *SendMessage, types, info string) {
	if types == "INFO" {
		if LogLevel != "INFO" {
			return
		}
		fmt.Fprintf(
			os.Stdout,
			"[SendInfo] %s %s %s | LOGTIME %s | RUNTIME %s %v %s | MSGID %s | EVENT %s %s %s | TOPIC %s %s %s | DATA %s | LOG %s\n",
			Green, types, Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			Green, time.Since(s.Context.StartTime), Reset,
			s.Context.Id,
			Yellow, s.Type, Reset,
			Cyan, s.Topic, Reset,
			string(s.Data),
			info)
	} else {
		fmt.Fprintf(
			os.Stderr,
			"[SendInfo] %s %s %s | LOGTIME %s | RUNTIME %s %v %s | MSGID %s | EVENT %s %s %s | TOPIC %s %s %s | DATA %s | LOG %s\n",
			Red, types, Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			Green, time.Since(s.Context.StartTime), Reset,
			s.Context.Id,
			Yellow, s.Type, Reset,
			Cyan, s.Topic, Reset,
			string(s.Data),
			info)
	}
}
