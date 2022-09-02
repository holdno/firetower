package redis

import "context"

type ClusterStore struct {
	provider *RedisProvider
}

func newClusterStore(provider *RedisProvider) *ClusterStore {
	return &ClusterStore{
		provider: provider,
	}
}

const (
	ClusterKey = "firetower_cluster_number"
)

func (s *ClusterStore) ClusterNumber() (int64, error) {
	res := s.provider.dbconn.Incr(context.TODO(), ClusterKey)
	return res.Val(), res.Err()
}
