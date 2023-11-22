package protocol

import (
	"sync"

	"go.uber.org/zap"
)

type Pusher interface {
	Publish(fire *FireInfo) error
	Receive() chan *FireInfo
}

type SinglePusher struct {
	msg      chan []byte
	b        Brazier
	once     sync.Once
	coder    Coder
	fireChan chan *FireInfo
	logger   *zap.Logger
}

func (s *SinglePusher) Publish(fire *FireInfo) error {
	s.msg <- s.coder.Encode(fire)
	return nil
}

func (s *SinglePusher) Receive() chan *FireInfo {
	s.once.Do(func() {
		go func() {
			for {
				select {
				case m := <-s.msg:
					fire := new(FireInfo)
					err := s.coder.Decode(m, fire)
					if err != nil {
						s.logger.Error("failed to decode message", zap.String("data", string(m)), zap.Error(err))
						continue
					}
					s.fireChan <- fire
				}
			}
		}()
	})
	return s.fireChan
}

type Brazier interface {
	Extinguished(fire *FireInfo)
	LightAFire() *FireInfo
}

func DefaultPusher(b Brazier, coder Coder, logger *zap.Logger) *SinglePusher {
	return &SinglePusher{
		msg:      make(chan []byte, 100),
		b:        b,
		coder:    coder,
		fireChan: make(chan *FireInfo, 10000),
	}
}
