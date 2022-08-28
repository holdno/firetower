package single

import (
	"sync"
)

type ClusterConnStore struct {
	storage map[string]uint64

	sync.RWMutex
}

func (s *ClusterConnStore) OneClientInc(clientIP string) error {
	s.Lock()
	defer s.Unlock()
	s.storage[clientIP] += 1
	return nil
}

func (s *ClusterConnStore) OneClientDec(clientIP string) error {
	s.Lock()
	defer s.Unlock()
	s.storage[clientIP] -= 1
	return nil
}

func (s *ClusterConnStore) GetAllConnNum() (uint64, error) {
	s.RLock()
	defer s.RUnlock()

	var result uint64
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
