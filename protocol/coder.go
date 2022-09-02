package protocol

import "github.com/vmihailenco/msgpack/v5"

type DefaultCoder struct{}

func (c *DefaultCoder) Decode(data []byte, fire *FireInfo) error {
	return msgpack.Unmarshal(data, fire)
}

func (c *DefaultCoder) Encode(msg *FireInfo) []byte {
	b, _ := msgpack.Marshal(msg)
	return b
}
