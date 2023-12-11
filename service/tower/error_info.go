package tower

import "errors"

var (
	// ErrorClosed gateway连接已经关闭的错误信息
	ErrorClosed = errors.New("firetower is collapsed")
	// ErrorTopicEmpty topic不存在的错误信息
	ErrorTopicEmpty = errors.New("topic is empty")
	// Server Side Mode Can not send to self
	ErrorServerSideMode = errors.New("server side tower can not send to self")
)
