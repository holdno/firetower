package tower

import (
	"github.com/holdno/firetower/protocol"
	"go.uber.org/zap"
)

type ServerSideTower interface {
	SetOnConnectHandler(fn func() bool)
	SetOnOfflineHandler(fn func())
	SetReceivedHandler(fn func(*protocol.FireInfo) bool)
	SetSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool)
	SetUnSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool)
	SetBeforeSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool)
	SetOnSystemRemove(fn func(topic string))
	GetConnectNum(topic string) (uint64, error)
	Publish(fire *protocol.FireInfo) error
	Subscribe(context protocol.FireLife, topics []string) error
	UnSubscribe(context protocol.FireLife, topics []string) error
	Logger() *zap.Logger
	TopicList() []string
	Run()
	Close()
	OnClose() chan struct{}
}

// BuildTower 实例化一个websocket客户端
func BuildServerSideTower(clientId string) ServerSideTower {
	return buildNewTower(nil, clientId)
}
