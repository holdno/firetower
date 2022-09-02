package single

type ClusterStore struct {
}

func (s *ClusterStore) ClusterNumber() (int64, error) {
	return 1, nil
}
