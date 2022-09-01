package protocol

const (
	// PublishKey 与前端(客户端约定的推送关键字)
	PublishKey = "publish"
	// OfflineTopicByUserIdKey 踢除，将用户某个topic踢下线
	OfflineTopicByUserIdKey = "offline_topic_by_userid"
	// OfflineTopicKey 针对某个topic进行踢除
	OfflineTopicKey = "offline_topic"
	// OfflineUserKey 将某个用户踢下线
	OfflineUserKey = "offline_user"
)
