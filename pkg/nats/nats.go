package nats

import (
	"sync"

	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/protocol"

	"github.com/nats-io/nats.go"
)

var _ protocol.Pusher = (*pusher)(nil)

func MustSetupNatsPusher(cfg config.Nats, b protocol.Brazier, coder protocol.Coder, topicFunc func() map[string]struct{}) protocol.Pusher {
	p := &pusher{
		b:            b,
		coder:        coder,
		currentTopic: topicFunc,
		msg:          make(chan *protocol.FireInfo, 10000),
	}
	var err error
	p.nats, err = nats.Connect(cfg.Addr, nats.Name(cfg.ServerName), nats.UserInfo(cfg.UserName, cfg.Password))
	if err != nil {
		panic(err)
	}
	return p
}

type pusher struct {
	msg          chan *protocol.FireInfo
	nats         *nats.Conn
	once         sync.Once
	b            protocol.Brazier
	coder        protocol.Coder
	currentTopic func() map[string]struct{}
}

func (p *pusher) Publish(fire *protocol.FireInfo) error {
	msg := nats.NewMsg("firetower.topic." + fire.Message.Topic)
	msg.Header.Set("topic", fire.Message.Topic)
	msg.Data = p.coder.Encode(fire)
	return p.nats.PublishMsg(msg)
}

func (p *pusher) Receive() chan *protocol.FireInfo {
	p.once.Do(func() {
		p.nats.Subscribe("firetower.topic.>", func(msg *nats.Msg) {
			topic := msg.Header.Get("topic")
			if _, exist := p.currentTopic()[topic]; exist {
				fire := new(protocol.FireInfo)
				if err := p.coder.Decode(msg.Data, fire); err != nil {
					// todo log
					return
				}
				p.msg <- fire
			}
		})
	})

	return p.msg
}
