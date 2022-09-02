package protocol

import "sync"

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
						// todo log
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

func DefaultPusher(b Brazier, coder Coder) *SinglePusher {
	return &SinglePusher{
		msg:      make(chan []byte, 100),
		b:        b,
		coder:    coder,
		fireChan: make(chan *FireInfo, 10000),
	}
}
