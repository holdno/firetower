package tower

import (
	"github.com/holdno/firetower/protocol"
)

type brazier struct {
}

func (b *brazier) Extinguished(fire *protocol.FireInfo) {
	fire.Info("Extinguished")
	fire.MessageType = 0
	fire.Message.Data = []byte("")
	fire.Message.Topic = ""
	fire.Message.Type = ""
	firePool.Put(fire)
}

func (b *brazier) LightAFire() *protocol.FireInfo {
	return firePool.Get().(*protocol.FireInfo)
}

type PusherInfo interface {
	ClientID() string
	UserID() string
}

func NewFire(source protocol.FireSource, tower PusherInfo) *protocol.FireInfo {
	f := tm.brazier.LightAFire()
	f.Message.Type = protocol.PublishKey
	f.Context.Reset(source, tower.ClientID(), tower.UserID())
	return f
}
