package redis

import (
	"context"
	"strconv"
	"sync"
)

type ClusterTopicStore struct {
	storage        map[string]int64
	provider       *RedisProvider
	clientTopicKey string

	sync.RWMutex
}

func newClusterTopicStore(provider *RedisProvider) *ClusterTopicStore {
	return &ClusterTopicStore{
		storage:  make(map[string]int64),
		provider: provider,
	}
}

const (
	ClusterTopicKeyPrefix = "ft_topics_conn_"
)

func (s *ClusterTopicStore) getTopicKey() string {
	if s.clientTopicKey == "" {
		s.clientTopicKey = ClusterTopicKeyPrefix + s.provider.clientIP
	}
	return s.clientTopicKey
}

func (s *ClusterTopicStore) TopicConnAtomicAddBy(topic string, num int64) error {
	s.Lock()
	defer s.Unlock()
	s.storage[topic] += num
	res := s.provider.dbconn.HSet(context.TODO(), s.getTopicKey(), topic, s.storage[topic])
	if res.Err() != nil {
		s.storage[topic] -= num
		return res.Err()
	}

	return nil
}

func (s *ClusterTopicStore) RemoveTopic(topic string) error {
	res := s.provider.dbconn.HDel(context.TODO(), s.getTopicKey(), topic)
	if res.Err() != nil {
		return res.Err()
	}
	s.Lock()
	defer s.Unlock()
	delete(s.storage, topic)
	return nil
}

func (s *ClusterTopicStore) GetTopicConnNum(topic string) (int64, error) {
	res := s.provider.dbconn.HGet(context.TODO(), s.getTopicKey(), topic)
	if res.Err() != nil {
		return 0, res.Err()
	}

	v, _ := strconv.ParseInt(res.Val(), 10, 64)
	return v, nil
}

func (s *ClusterTopicStore) Topics() ([]string, error) {
	res := s.provider.dbconn.HGetAll(context.TODO(), s.getTopicKey())
	if res.Err() != nil {
		return nil, res.Err()
	}

	var list []string
	for k := range res.Val() {
		list = append(list, k)
	}

	return list, nil
}
