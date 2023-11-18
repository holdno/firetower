package redis

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

type ClusterConnStore struct {
	storage  map[string]int64
	provider *RedisProvider
	isMaster bool

	keepMasterScriptSHA             string
	clusterShutdownCheckerScriptSHA string

	sync.RWMutex
}

const (
	ClusterConnKey = "ft_cluster_conn"
)

func newClusterConnStore(provider *RedisProvider) *ClusterConnStore {
	s := &ClusterConnStore{
		storage:  make(map[string]int64),
		provider: provider,
	}

	res := s.provider.dbconn.ScriptLoad(context.TODO(), clusterShutdownCheckerScript)
	if res.Err() != nil {
		panic(res.Err())
	}
	s.clusterShutdownCheckerScriptSHA = res.Val()

	res = s.provider.dbconn.ScriptLoad(context.TODO(), keepMasterScript)
	if res.Err() != nil {
		panic(res.Err())
	}
	s.keepMasterScriptSHA = res.Val()

	go s.KeepClusterClear()

	if err := s.init(); err != nil {
		panic(err)
	}
	return s
}

func (s *ClusterConnStore) init() error {
	res := s.provider.dbconn.HDel(context.TODO(), s.provider.keyPrefix+ClusterConnKey, s.provider.clientIP)
	return res.Err()
}

func (s *ClusterConnStore) ClusterMembers() ([]string, error) {
	res := s.provider.dbconn.HGetAll(context.TODO(), s.provider.keyPrefix+ClusterConnKey)
	if res.Err() != nil {
		return nil, res.Err()
	}
	var list []string
	for k := range res.Val() {
		list = append(list, k)
	}
	return list, nil
}

func packClientConnNumberNow(num uint64) string {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, num)
	binary.LittleEndian.PutUint64(b[8:], uint64(time.Now().Unix()))
	return string(b)
}

func unpackClientConnNumberNow(b string) (uint64, error) {
	if len([]byte(b)) != 16 {
		return 0, fmt.Errorf("wrong pack data, got lenght %d, need 16", len([]byte(b)))
	}
	return binary.LittleEndian.Uint64([]byte(b)[:8]), nil
}

func (s *ClusterConnStore) OneClientAtomicAddBy(clientIP string, num int64) error {
	s.Lock()
	defer s.Unlock()
	s.storage[clientIP] += num

	res := s.provider.dbconn.HSet(context.TODO(), s.provider.keyPrefix+ClusterConnKey, clientIP, packClientConnNumberNow(uint64(s.storage[clientIP])))
	if res.Err() != nil {
		s.storage[clientIP] -= num
		return res.Err()
	}
	return nil
}

func (s *ClusterConnStore) GetAllConnNum() (uint64, error) {
	res := s.provider.dbconn.HGetAll(context.TODO(), s.provider.keyPrefix+ClusterConnKey)
	if res.Err() != nil {
		return 0, res.Err()
	}
	var result uint64
	for _, v := range res.Val() {
		singleNumber, err := unpackClientConnNumberNow(v)
		if err != nil {
			return 0, err
		}
		result += singleNumber
	}
	return result, nil
}

func (s *ClusterConnStore) RemoveClient(clientIP string) error {
	res := s.provider.dbconn.HDel(context.TODO(), s.provider.keyPrefix+ClusterConnKey, clientIP)
	if res.Err() != nil {
		return res.Err()
	}
	s.Lock()
	defer s.Unlock()
	delete(s.storage, clientIP)

	return nil
}

const clusterShutdownCheckerScript = `
local key = KEYS[1]
local currentTime = KEYS[2]
local clientConns = redis.call("hgetall", key)
local delTable = {}

for i = 1, #clientConns, 2 do
	local number, timestamp = struct.unpack("<i8<i8",clientConns[i+1])
	if (currentTime > tostring(timestamp + 5))
	then 
		table.insert(delTable, clientConns[i])
	end
end

return redis.call("hdel", key, unpack(delTable))
`

func (s *ClusterConnStore) KeepClusterClear() {
	go s.SelectMaster()
	for {
		time.Sleep(time.Second)

		if !s.isMaster {
			continue
		}

		result := s.provider.dbconn.EvalSha(context.TODO(), s.clusterShutdownCheckerScriptSHA, []string{s.provider.keyPrefix + ClusterConnKey, fmt.Sprintf("%d", time.Now().Unix())}, 2)
		if result.Err() != nil {
			// todo log
			continue
		}
	}
}

const keepMasterScript = `
local lockKey = KEYS[1]
local currentID = KEYS[2]
local currentMaster = redis.call('get', lockKey)
local success = "fail"

if (currentID == currentMaster) 
then
	redis.call('expire', lockKey, 3)
	success = "success"
end
return success
`

func (s *ClusterConnStore) SelectMaster() {
	lockKey := s.provider.keyPrefix + "ft_cluster_master"
	for {
		res := s.provider.dbconn.SetNX(context.Background(), lockKey, s.provider.clientIP, time.Second*3)
		if res.Val() {
			s.isMaster = true
			ticker := time.NewTicker(time.Second * 1)
			for {
				select {
				case <-ticker.C:
					res := s.provider.dbconn.EvalSha(context.Background(), s.keepMasterScriptSHA, []string{lockKey, s.provider.clientIP}, 2)
					if res.Val() != "success" || res.Err() != nil {
						s.isMaster = false
						ticker.Stop()
						break
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
}
