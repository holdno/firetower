package socket

import (
	"fmt"
	"io"
	"os"
	"time"
)

var (
	LogLevel                     = "INFO"
	DefaultWriter      io.Writer = os.Stdout
	DefaultErrorWriter io.Writer = os.Stderr
	SendLogger         func(s *SendMessage, types, info string)
	// log color
	Green   = string([]byte{27, 91, 57, 55, 59, 52, 50, 109})
	White   = string([]byte{27, 91, 57, 48, 59, 52, 55, 109})
	Yellow  = string([]byte{27, 91, 57, 55, 59, 52, 51, 109})
	Red     = string([]byte{27, 91, 57, 55, 59, 52, 49, 109})
	Blue    = string([]byte{27, 91, 57, 55, 59, 52, 52, 109})
	Magenta = string([]byte{27, 91, 57, 55, 59, 52, 53, 109})
	Cyan    = string([]byte{27, 91, 57, 55, 59, 52, 54, 109})
	Reset   = string([]byte{27, 91, 48, 109})
)

func (s *SendMessage) Recycling() {
	sendPool.Put(s)
}

func (s *SendMessage) Panic(info string) {
	SendLogger(s, "Panic", info)
	s.Recycling()
}

func (s *SendMessage) Info(info string) {
	SendLogger(s, "INFO", info)
}

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
