package single

import "github.com/holdno/firetower/store"

var provider *SingleProvider

type SingleProvider struct {
	clusterConnStore  store.ClusterConnStore
	clusterTopicStore store.ClusterTopicStore
	clusterStore      store.ClusterStore
}

func Setup() (*SingleProvider, error) {
	provider = &SingleProvider{
		clusterConnStore: &ClusterConnStore{
			storage: make(map[string]int64),
		},
		clusterTopicStore: &ClusterTopicStore{
			storage: make(map[string]int64),
		},
		clusterStore: &ClusterStore{},
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

func (s *SingleProvider) ClusterStore() store.ClusterStore {
	return s.clusterStore
}
