package startup

import "sync"

// Use Pools to reuse chans
var connChPool sync.Pool
var reqChPool sync.Pool
var resChPool sync.Pool

func init() {
	connChPool.New = func() any {
		return make(chan *Connection, 1)
	}

	reqChPool.New = func() any {
		return make(chan *ReqWithChan, 1)
	}

	resChPool.New = func() any {
		return make(chan *Res, 1)
	}
}
