package store

import (
	"sync"
)

var (
	TopicTable      sync.Map // map[int][]*beaconTower // 话题->wsid索引表
	TowerIndexTable sync.Map // map[int]beaconTower    // wsid->ws索引表

	IP string
)
