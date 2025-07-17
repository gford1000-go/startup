package startup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Location is the type of chan to establish a connection with an Identity
type Location chan<- *Connect

// SendOptions allow further configuration when sending requests
type SendOptions struct {
	// Timeout is the maximum time the requestor will wait for a response
	Timeout time.Duration
}

// ConnectOptions all further configuration when initiating connections
type ConnectOptions struct {
	// Timeout is the maximum time the initiator will wait for the remote identity to respond
	Timeout time.Duration
	// DisoveryService specifies the DiscoveryService to use when retrieving remote identities
	DisoveryService DiscoveryService
}

// Identity ties an ID with the means to connect to that ID
type Identity interface {
	// ID is unique for each identity
	ID() string
	// Loc is the initiating Location for a Connection to be established
	Loc() Location
	// Accept allows an Identity to respond to Connection attempts
	Accept(context.Context)
	// Allows an Identity to connect to another Identity specified by the id
	Connect(ctx context.Context, id string, opts ...func(*ConnectOptions)) (*Connection, error)
	// Send allows an Identity to make a request to the remote identity, after Connection is established
	Send(ctx context.Context, r *Req, ch chan<- *ReqWithChan, opts ...func(*SendOptions)) *Res
}

// CreateAndRegisterID creates an Identity and attempts to register it on the DiscoveryService
func CreateAndRegisterID(id string, d time.Duration, h Handler, ds DiscoveryService) (Identity, error) {
	i := &identity{
		id:          id,
		ch:          make(chan *Connect),
		h:           h,
		idleTimeout: d,
	}
	if err := ds.Register(i); err != nil {
		return nil, fmt.Errorf("%s already exists!: %v", id, err)
	}
	return i, nil
}

// Handler will process a Req to populate a Res
type Handler func(context.Context, *Req, *Res)

// identity is an implementation of Identity
type identity struct {
	id          string
	ch          chan *Connect
	h           Handler
	idleTimeout time.Duration
}

func (i *identity) ID() string {
	return i.id
}

func (i *identity) Loc() Location {
	return i.ch
}

func (i *identity) Accept(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case c, ok := <-i.ch:
			if !ok {
				return
			}
			// For now, ignore ID
			ch := reqChPool.Get().(chan *ReqWithChan)
			go i.handle(ctx, ch)

			c.Chan <- &Connection{
				ReqChan: ch,
				Timeout: i.idleTimeout,
			}
		}
	}
}

func (i *identity) handle(ctx context.Context, ch chan *ReqWithChan) {
	defer reqChPool.Put(ch)

	hWrapper := func(req *Req) (res *Res) {
		res = &Res{}
		defer func() {
			if r := recover(); r != nil {
				res.Status = Error
				res.Error = fmt.Errorf("caught panic: %v", r)
				res.Type = ""
				res.Data = nil
			}
		}()

		i.h(ctx, req, res)
		return res
	}

	for {
		select {
		case <-ctx.Done():
			return
		case r, ok := <-ch:
			if !ok {
				return
			}
			r.Chan <- hWrapper(&Req{Type: r.Type, Data: r.Data})
		case <-time.After(i.idleTimeout):
			return
		}
	}
}

// ErrContextCompleted returned if the context has completed, indicating shutdown
var ErrContextCompleted = errors.New("context completed")

// ErrConnectionChan returned when there is an issue with Connection
var ErrConnectionChan = errors.New("connection chan error")

// ErrNilConnection returned when the remote Identity does not return Connection info
var ErrNilConnection = errors.New("nil was returned by remote")

// ErrConnectTimeout returned if the remote Identity does not respond within the specified timeout
var ErrConnectTimeout = errors.New("timeout whilst attempting connect")

var defaultConnectOptions = ConnectOptions{
	Timeout: 10 * time.Second,
}

// WithConnectTimeout overrides the default timeout for Connect to complete
func WithConnectTimeout(d time.Duration) func(*ConnectOptions) {
	return func(co *ConnectOptions) {
		if d > 0 {
			co.Timeout = d
		}
	}
}

// WithConnectDiscoveryService overrides the default DiscoveryService
func WithConnectDiscoveryService(ds DiscoveryService) func(*ConnectOptions) {
	return func(co *ConnectOptions) {
		if ds == nil {
			panic("nil provided to WithConnectDiscoveryService()")
		}
		co.DisoveryService = ds
	}
}

// ErrNoDiscoveryService returned when a DiscoveryService is not specified (there is no default service)
var ErrNoDiscoveryService = errors.New("cannot connect, no Discovery Service available")

func (i *identity) Connect(ctx context.Context, id string, opts ...func(*ConnectOptions)) (*Connection, error) {

	var o ConnectOptions = defaultConnectOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.DisoveryService == nil {
		return nil, ErrNoDiscoveryService
	}

	loc, err := o.DisoveryService.Find(id)
	if err != nil {
		return nil, ErrIDNotFound
	}

	ch := connChPool.Get().(chan *Connection)
	defer connChPool.Put(ch)

	loc <- &Connect{
		ReqID: i.id,
		Chan:  ch,
	}

	select {
	case <-ctx.Done():
		return nil, ErrContextCompleted
	case <-time.After(o.Timeout):
		return nil, ErrConnectTimeout
	case c, ok := <-ch:
		if !ok {
			return nil, ErrConnectionChan
		}
		if c == nil {
			return nil, ErrNilConnection
		}
		return c, nil
	}
}

var defaultSendOptions = SendOptions{
	Timeout: time.Hour,
}

// WithSendTimeout overrides the default timeout for receiving a Res to a Send
func WithSendTimeout(d time.Duration) func(*SendOptions) {
	return func(so *SendOptions) {
		so.Timeout = d
	}
}

func (i *identity) Send(ctx context.Context, req *Req, ch chan<- *ReqWithChan, opts ...func(*SendOptions)) (r *Res) {

	var o SendOptions = defaultSendOptions
	for _, opt := range opts {
		opt(&o)
	}

	rCh := resChPool.Get().(chan *Res)
	defer func() {
		if r != nil && r.Status == RequestTimeout {
			<-rCh // Possible corruption if we don't wait for response, given pool reuse of the chans
		}
		resChPool.Put(rCh)
	}()

	ch <- &ReqWithChan{
		Req: Req{
			Type: req.Type,
			Data: req.Data,
		},
		Chan: rCh,
	}

	select {
	case <-ctx.Done():
		resChPool.Put(rCh)
		return nil
	case r, ok := <-rCh:
		if !ok {
			return nil
		}
		return r
	case <-time.After(o.Timeout):
		return &Res{
			Status: RequestTimeout,
			Error:  errors.New("timeout"),
		}
	}
}
