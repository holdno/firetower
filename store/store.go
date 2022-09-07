package store

type ClusterConnStore interface {
	OneClientAtomicAddBy(clientIP string, num int64) error
	GetAllConnNum() (uint64, error)
	RemoveClient(clientIP string) error
	ClusterMembers() ([]string, error)
}

type ClusterTopicStore interface {
	TopicConnAtomicAddBy(topic string, num int64) error
	GetTopicConnNum(topic string) (uint64, error)
	RemoveTopic(topic string) error
	Topics() (map[string]uint64, error)
	ClusterTopics() (map[string]uint64, error)
}

type ClusterStore interface {
	ClusterNumber() (int64, error)
}
