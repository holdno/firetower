package single

import (
	"sync"
)

type ClusterConnStore struct {
	storage map[string]int64

	sync.RWMutex
}

func (s *ClusterConnStore) ClusterMembers() ([]string, error) {
	var list []string
	for k := range s.storage {
		list = append(list, k)
	}
	return list, nil
}

func (s *ClusterConnStore) OneClientAtomicAddBy(clientIP string, num int64) error {
	s.Lock()
	defer s.Unlock()
	s.storage[clientIP] += num
	return nil
}

func (s *ClusterConnStore) GetAllConnNum() (int64, error) {
	s.RLock()
	defer s.RUnlock()

	var result int64
	for _, v := range s.storage {
		result += v
	}
	return result, nil
}

func (s *ClusterConnStore) RemoveClient(clientIP string) error {
	s.Lock()
	defer s.Unlock()

	delete(s.storage, clientIP)
	return nil
}
