package tower

import (
	"github.com/holdno/firetower/protocol"
)

type brazier struct {
	len int
}

func (b *brazier) Extinguished(fire *protocol.FireInfo) {
	if b.len > 100000 {
		return
	}
	b.len++
	fire.MessageType = 0
	fire.Message.Data = []byte("")
	fire.Message.Topic = ""
	fire.Message.Type = 0
	firePool.Put(fire)
}

func (b *brazier) LightAFire() *protocol.FireInfo {
	b.len--
	return firePool.Get().(*protocol.FireInfo)
}

type PusherInfo interface {
	ClientID() string
	UserID() string
}

func NewFire(source protocol.FireSource, tower PusherInfo) *protocol.FireInfo {
	f := tm.brazier.LightAFire()
	f.Message.Type = protocol.PublishOperation
	f.Context.Reset(source, tower.ClientID(), tower.UserID())
	return f
}
