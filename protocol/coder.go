package protocol

import "github.com/vmihailenco/msgpack/v5"

type DefaultCoder[T any] struct{}

func (c *DefaultCoder[T]) Decode(data []byte, fire *FireInfo[T]) error {
	return msgpack.Unmarshal(data, fire)
}

func (c *DefaultCoder[T]) Encode(msg *FireInfo[T]) []byte {
	b, _ := msgpack.Marshal(msg)
	return b
}
