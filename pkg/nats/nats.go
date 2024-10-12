package nats

import (
	"log/slog"
	"sync"

	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/protocol"

	"github.com/nats-io/nats.go"
)

var _ protocol.Pusher[any] = (*pusher[any])(nil)

func MustSetupNatsPusher[T any](cfg config.Nats, coder protocol.Coder[T], logger protocol.Logger, topicFunc func() map[string]uint64) protocol.Pusher[T] {
	if cfg.SubjectPrefix == "" {
		cfg.SubjectPrefix = "firetower.topic."
	}
	p := &pusher[T]{
		subjectPrefix: cfg.SubjectPrefix,
		coder:         coder,
		currentTopic:  topicFunc,
		msg:           make(chan *protocol.FireInfo[T], 10000),
		logger:        logger,
	}
	var err error
	p.nats, err = nats.Connect(cfg.Addr, nats.Name(cfg.ServerName), nats.UserInfo(cfg.UserName, cfg.Password))
	if err != nil {
		panic(err)
	}
	return p
}

type pusher[T any] struct {
	subjectPrefix string
	msg           chan *protocol.FireInfo[T]
	nats          *nats.Conn
	once          sync.Once
	b             protocol.Brazier[T]
	coder         protocol.Coder[T]
	currentTopic  func() map[string]uint64
	logger        protocol.Logger
}

func (p *pusher[T]) Publish(fire *protocol.FireInfo[T]) error {
	msg := nats.NewMsg(p.subjectPrefix + fire.Message.Topic)
	msg.Header.Set("topic", fire.Message.Topic)
	msg.Data = p.coder.Encode(fire)
	return p.nats.PublishMsg(msg)
}

func (p *pusher[T]) Receive() chan *protocol.FireInfo[T] {
	p.once.Do(func() {
		p.nats.Subscribe(p.subjectPrefix+">", func(msg *nats.Msg) {
			topic := msg.Header.Get("topic")
			if _, exist := p.currentTopic()[topic]; exist {
				fire := new(protocol.FireInfo[T])
				if err := p.coder.Decode(msg.Data, fire); err != nil {
					p.logger.Error("failed to decode message", slog.String("data", string(msg.Data)), slog.Any("error", err), slog.String("topic", topic))
					return
				}
				p.msg <- fire
			}
			msg.Ack()
		})
	})

	return p.msg
}
