package socket

import "errors"

var (
	ErrorClose = errors.New("beacon tower is collapsed")
	ErrorBlock = errors.New("network congestion")
)
