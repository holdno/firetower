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

func (s *ClusterTopicStore) GetTopicConnNum(topic string) (int64, error) {
	s.RLock()
	defer s.RUnlock()
	return s.storage[topic], nil
}

func (s *ClusterTopicStore) Topics() ([]string, error) {
	s.RLock()
	defer s.RUnlock()
	var list []string
	for k := range s.storage {
		list = append(list, k)
	}
	return list, nil
}
