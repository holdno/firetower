package redis

import (
	"context"

	"github.com/go-redis/redis/v9"
	"github.com/holdno/firetower/store"
)

var provider *RedisProvider

type RedisProvider struct {
	dbconn    *redis.Client
	clientIP  string
	keyPrefix string

	clusterConnStore  store.ClusterConnStore
	clusterTopicStore store.ClusterTopicStore
	clusterStore      store.ClusterStore
}

func Setup(addr, password string, db int, clientIP, keyPrefix string) (*RedisProvider, error) {
	if clientIP == "" {
		panic("setup redis store: required client IP")
	}
	provider = &RedisProvider{
		keyPrefix: keyPrefix,
		dbconn: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password, // no password set
			DB:       db,       // use default DB
		}),
		clientIP: clientIP,
	}

	provider.clusterConnStore = newClusterConnStore(provider)
	provider.clusterTopicStore = newClusterTopicStore(provider)
	provider.clusterStore = newClusterStore(provider)

	res := provider.dbconn.Ping(context.TODO())
	if res.Err() != nil {
		return nil, res.Err()
	}

	return provider, nil
}

func (s *RedisProvider) ClusterConnStore() store.ClusterConnStore {
	return s.clusterConnStore
}

func (s *RedisProvider) ClusterTopicStore() store.ClusterTopicStore {
	return s.clusterTopicStore
}

func (s *RedisProvider) ClusterStore() store.ClusterStore {
	return s.clusterStore
}
