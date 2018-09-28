package socket

import "errors"

var (
	// 连接关闭的错误信息
	ErrorClose = errors.New("firetower is collapsed")
	// block错误信息
	ErrorBlock = errors.New("network congestion")
)
