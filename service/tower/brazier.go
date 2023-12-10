package tower

import (
	"sync"

	"github.com/holdno/firetower/protocol"
)

func newBrazier[T any]() *brazier[T] {
	return &brazier[T]{
		pool: &sync.Pool{
			New: func() interface{} {
				return &protocol.FireInfo[T]{
					Context: protocol.FireLife{},
					Message: protocol.TopicMessage[T]{},
				}
			},
		},
	}
}

type brazier[T any] struct {
	len  int
	pool *sync.Pool
}

func (b *brazier[T]) Extinguished(fire *protocol.FireInfo[T]) {
	if fire == nil || b.len > 100000 {
		return
	}
	var empty T
	b.len++
	fire.MessageType = 0
	fire.Message.Data = empty
	fire.Message.Topic = ""
	fire.Message.Type = 0
	b.pool.Put(fire)
}

func (b *brazier[T]) LightAFire() *protocol.FireInfo[T] {
	b.len--
	return b.pool.Get().(*protocol.FireInfo[T])
}

type PusherInfo interface {
	ClientID() string
	UserID() string
}

func (t *TowerManager[T]) NewFire(source protocol.FireSource, tower PusherInfo) *protocol.FireInfo[T] {
	f := t.brazier.LightAFire()
	f.Message.Type = protocol.PublishOperation
	f.Context.Reset(source, tower.ClientID(), tower.UserID())
	return f
}
