package store

type ClusterConnStore interface {
	OneClientAtomicAddBy(clientIP string, num int64) error
	GetAllConnNum() (int64, error)
	RemoveClient(clientIP string) error
}

type ClusterTopicStore interface {
	TopicConnAtomicAddBy(topic string, num int64) error
	GetTopicConnNum(topic string) (int64, error)
	RemoveTopic(topic string) error
	Topics() ([]string, error)
}

type ClusterStore interface {
	ClusterNumber() (int64, error)
}
