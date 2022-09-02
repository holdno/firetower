package redis

import (
	"context"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type ClusterConnStore struct {
	storage  map[string]int64
	provider *RedisProvider

	sync.RWMutex
}

const (
	ClusterConnKey = "ft_cluster_conn"
)

func newClusterConnStore(provider *RedisProvider) *ClusterConnStore {
	return &ClusterConnStore{
		storage:  make(map[string]int64),
		provider: provider,
	}
}

func (s *ClusterConnStore) ClusterMembers() ([]string, error) {
	res := s.provider.dbconn.HGetAll(context.TODO(), ClusterConnKey)
	if res.Err() != nil {
		return nil, res.Err()
	}
	var list []string
	for k := range res.Val() {
		list = append(list, k)
	}
	return list, nil
}

// N: connNum, T: updateTime
type connNumTmp struct {
	N int64
	T int64
}

const connNumJsonTmp = `{"n": %d,"t": %d}`

func (s *ClusterConnStore) OneClientAtomicAddBy(clientIP string, num int64) error {
	s.Lock()
	defer s.Unlock()
	s.storage[clientIP] += num
	b, _ := msgpack.Marshal(&connNumTmp{
		N: s.storage[clientIP],
		T: time.Now().Unix(),
	})
	res := s.provider.dbconn.HSet(context.TODO(), ClusterConnKey, clientIP, string(b))
	if res.Err() != nil {
		s.storage[clientIP] -= num
		return res.Err()
	}
	return nil
}

func (s *ClusterConnStore) GetAllConnNum() (int64, error) {
	res := s.provider.dbconn.HGetAll(context.TODO(), ClusterConnKey)
	if res.Err() != nil {
		return 0, res.Err()
	}
	var result int64
	for _, v := range res.Val() {
		var value connNumTmp
		if err := msgpack.Unmarshal([]byte(v), &value); err != nil {
			return 0, err
		}
		result += value.N
	}
	return result, nil
}

func (s *ClusterConnStore) RemoveClient(clientIP string) error {
	res := s.provider.dbconn.HDel(context.TODO(), ClusterConnKey, clientIP)
	if res.Err() != nil {
		return res.Err()
	}
	s.Lock()
	defer s.Unlock()
	delete(s.storage, clientIP)

	return nil
}
