package protocol

type FireSource uint8

const (
	SourceClient FireSource = 1
	SourceLogic  FireSource = 2
	SourceSystem FireSource = 3
)

func (s FireSource) String() string {
	switch s {
	case SourceClient:
		return "client"
	case SourceLogic:
		return "logic"
	case SourceSystem:
		return "system"
	default:
		return "unknown"
	}
}

type FireOperation uint8

const (
	// PublishKey 与前端(客户端约定的推送关键字)
	PublishOperation     FireOperation = 1
	SubscribeOperation   FireOperation = 2
	UnSubscribeOperation FireOperation = 3
	// OfflineTopicByUserIdKey 踢除，将用户某个topic踢下线
	OfflineTopicByUserIdOperation FireOperation = 4
	// OfflineTopicKey 针对某个topic进行踢除
	OfflineTopicOperation FireOperation = 5
	// OfflineUserKey 将某个用户踢下线
	OfflineUserOperation FireOperation = 6
)

func (o FireOperation) String() string {
	switch o {
	case PublishOperation:
		return "publish"
	case SubscribeOperation:
		return "subscribe"
	case UnSubscribeOperation:
		return "unSubscribe"
	case OfflineTopicByUserIdOperation:
		return "offlineTopicByUserid"
	case OfflineTopicOperation:
		return "offlineTopic"
	case OfflineUserOperation:
		return "offlineUser"
	default:
		return "unknown"
	}
}

const (
	SYSTEM_CMD_REMOVE_USER = "remove_user"
)
