package socket

import "errors"

var (
	// ErrorClose 连接关闭的错误信息
	ErrorClose = errors.New("firetower is collapsed")
	// ErrorBlock block错误信息
	ErrorBlock = errors.New("network congestion")
)
