package single

import (
	"sync"
)

type ClusterTopicStore struct {
	storage map[string]int64

	sync.RWMutex
}

func (s *ClusterTopicStore) TopicConnAtomicAddBy(topic string, num int64) error {
	s.Lock()
	defer s.Unlock()

	s.storage[topic] += num
	return nil
}

func (s *ClusterTopicStore) RemoveTopic(topic string) error {
	s.Lock()
	defer s.Unlock()
	delete(s.storage, topic)
	return nil
}

func (s *ClusterTopicStore) GetTopicConnNum(topic string) (uint64, error) {
	s.RLock()
	defer s.RUnlock()
	return uint64(s.storage[topic]), nil
}

func (s *ClusterTopicStore) Topics() (map[string]uint64, error) {
	result := make(map[string]uint64)
	for k, v := range result {
		result[k] = uint64(v)
	}
	return result, nil
}

func (s *ClusterTopicStore) ClusterTopics() (map[string]uint64, error) {
	return s.Topics()
}
