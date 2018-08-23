package socket

import "errors"

var (
	ErrorClose = errors.New("firetower is collapsed")
	ErrorBlock = errors.New("network congestion")
)
