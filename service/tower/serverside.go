package tower

import (
	"github.com/holdno/firetower/protocol"
)

type ServerSideTower[T any] interface {
	SetOnConnectHandler(fn func() bool)
	SetOnOfflineHandler(fn func())
	SetReceivedHandler(fn func(protocol.ReadOnlyFire[T]) bool)
	SetSubscribeHandler(fn func(context protocol.FireLife, topic []string))
	SetUnSubscribeHandler(fn func(context protocol.FireLife, topic []string))
	SetBeforeSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool)
	SetOnSystemRemove(fn func(topic string))
	GetConnectNum(topic string) (uint64, error)
	Publish(fire *protocol.FireInfo[T]) error
	Subscribe(context protocol.FireLife, topics []string) error
	UnSubscribe(context protocol.FireLife, topics []string) error
	Logger() protocol.Logger
	TopicList() []string
	Run()
	Close()
	OnClose() chan struct{}
}

// BuildTower 实例化一个websocket客户端
func (t *TowerManager[T]) BuildServerSideTower(clientId string) ServerSideTower[T] {
	return buildNewTower(t, nil, clientId)
}
