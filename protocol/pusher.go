package protocol

import (
	"sync"

	"go.uber.org/zap"
)

type Pusher[T any] interface {
	Publish(fire *FireInfo[T]) error
	Receive() chan *FireInfo[T]
}

type SinglePusher[T any] struct {
	msg      chan []byte
	b        Brazier[T]
	once     sync.Once
	coder    Coder[T]
	fireChan chan *FireInfo[T]
	logger   *zap.Logger
}

func (s *SinglePusher[T]) Publish(fire *FireInfo[T]) error {
	s.msg <- s.coder.Encode(fire)
	return nil
}

func (s *SinglePusher[T]) Receive() chan *FireInfo[T] {
	s.once.Do(func() {
		go func() {
			for {
				select {
				case m := <-s.msg:
					fire := new(FireInfo[T])
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

type Brazier[T any] interface {
	Extinguished(fire *FireInfo[T])
	LightAFire() *FireInfo[T]
}

func DefaultPusher[T any](b Brazier[T], coder Coder[T], logger *zap.Logger) *SinglePusher[T] {
	return &SinglePusher[T]{
		msg:      make(chan []byte, 100),
		b:        b,
		coder:    coder,
		fireChan: make(chan *FireInfo[T], 10000),
	}
}
