package store

import (
	"sync"
)

var (
	TopicTable      sync.Map // map[string(topic)][]string(websocket client id) // 话题->wsid索引表
	TowerIndexTable sync.Map // map[string(websocket client id)]beaconTower(websocket conn)    // wsid->ws索引表

	IP string
)
