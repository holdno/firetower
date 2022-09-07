package redis

import (
	"context"
	"strconv"
	"sync"

	"github.com/holdno/firetower/store"
)

type ClusterTopicStore struct {
	storage  map[string]int64
	provider *RedisProvider

	sync.RWMutex
}

func newClusterTopicStore(provider *RedisProvider) store.ClusterTopicStore {
	s := &ClusterTopicStore{
		storage:  make(map[string]int64),
		provider: provider,
	}
	if err := s.init(); err != nil {
		panic(err)
	}
	return s
}

const (
	ClusterTopicKeyPrefix = "ft_topics_conn_"
)

func (s *ClusterTopicStore) getTopicKey(clientIP string) string {
	return ClusterTopicKeyPrefix + clientIP
}

func (s *ClusterTopicStore) init() error {
	res := s.provider.dbconn.Del(context.TODO(), s.getTopicKey(s.provider.clientIP))
	return res.Err()
}

func (s *ClusterTopicStore) TopicConnAtomicAddBy(topic string, num int64) error {
	s.Lock()
	defer s.Unlock()
	s.storage[topic] += num
	res := s.provider.dbconn.HSet(context.TODO(), s.getTopicKey(s.provider.clientIP), topic, s.storage[topic])
	if res.Err() != nil {
		s.storage[topic] -= num
		return res.Err()
	}

	if s.storage[topic] == 0 {
		res := s.provider.dbconn.HDel(context.TODO(), s.getTopicKey(s.provider.clientIP), topic)
		if res.Err() != nil {
			return res.Err()
		}
	}

	return nil
}

func (s *ClusterTopicStore) RemoveTopic(topic string) error {
	res := s.provider.dbconn.HDel(context.TODO(), s.getTopicKey(s.provider.clientIP), topic)
	if res.Err() != nil {
		return res.Err()
	}
	s.Lock()
	defer s.Unlock()
	delete(s.storage, topic)
	return nil
}

func (s *ClusterTopicStore) GetTopicConnNum(topic string) (uint64, error) {
	members, err := s.provider.ClusterConnStore().ClusterMembers()
	if err != nil {
		return 0, err
	}

	var total uint64
	for _, v := range members {
		res := s.provider.dbconn.HGet(context.TODO(), s.getTopicKey(v), topic)
		if res.Err() != nil {
			return 0, res.Err()
		}
		num, _ := strconv.ParseUint(res.Val(), 10, 64)
		total += num
	}

	return total, nil
}

func (s *ClusterTopicStore) Topics() (map[string]uint64, error) {
	res := s.provider.dbconn.HGetAll(context.TODO(), s.getTopicKey(s.provider.clientIP))
	if res.Err() != nil {
		return nil, res.Err()
	}

	result := make(map[string]uint64)
	for k, v := range res.Val() {
		vInt, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, err
		}
		result[k] = vInt
	}

	return result, nil
}

func (s *ClusterTopicStore) ClusterTopics() (map[string]uint64, error) {
	members, err := s.provider.ClusterConnStore().ClusterMembers()
	if err != nil {
		return nil, err
	}

	result := make(map[string]uint64)
	for _, v := range members {
		res := s.provider.dbconn.HGetAll(context.TODO(), s.getTopicKey(v))
		if res.Err() != nil {
			return nil, res.Err()
		}
		for k, v := range res.Val() {
			vInt, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, err
			}
			result[k] += vInt
		}
	}

	return result, nil
}
