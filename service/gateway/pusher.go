package gateway

import "github.com/holdno/firetower/protocol"

type SinglePusher struct {
}

func (s *SinglePusher) Publish(fire *protocol.FireInfo) error {
	tm.centralChan <- fire
	return nil
}
