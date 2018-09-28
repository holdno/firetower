package gateway

import "errors"

var (
	// gateway连接已经关闭的错误信息
	ErrorClose = errors.New("firetower is collapsed")
	// topic不存在的错误信息
	ErrorTopicEmpty = errors.New("topic is empty")
)
