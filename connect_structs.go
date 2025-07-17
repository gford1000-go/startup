package startup

import "time"

// Connection is returned by the Remote when a Requestor connects to it
type Connection struct {
	// ReqChan is the chan that the Remote indicates subsequent requests should be made to, for this Requestor
	ReqChan chan<- *ReqWithChan
	// Timeout is the duration after which the Connection will be dropped by the Remote
	Timeout time.Duration
}

// Connect is the initial information sent by the Requestor to the Remote.
// The requestor identifies themselves, and provides a chan on which the Remote can respond
type Connect struct {
	ReqID string
	Chan  chan<- *Connection
}
