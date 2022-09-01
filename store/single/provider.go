package single

import "github.com/holdno/firetower/store"

var provider *SingleProvider

type SingleProvider struct {
	clusterConnStore  store.ClusterConnStore
	clusterTopicStore store.ClusterTopicStore
}

func Setup() (*SingleProvider, error) {
	provider = &SingleProvider{
		clusterConnStore: &ClusterConnStore{
			storage: make(map[string]int64),
		},
		clusterTopicStore: &ClusterTopicStore{
			storage: make(map[string]int64),
		},
	}
	return provider, nil
}

func Provider() *SingleProvider {
	return provider
}

func (s *SingleProvider) ClusterConnStore() store.ClusterConnStore {
	return s.clusterConnStore
}

func (s *SingleProvider) ClusterTopicStore() store.ClusterTopicStore {
	return s.clusterTopicStore
}
