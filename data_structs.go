package startup

// Req is an individual Req for the Requestor to the Remote.
// The request describes the type of the data (encoded as JSON)
type Req struct {
	Type string
	Data any
}

// ReqWithChan provides the chan on which the Requestor is expecting the Res
type ReqWithChan struct {
	Req
	Chan chan<- *Res
}

// Status specifies whether the Req was handled ok
type Status int

const (
	UnknownStatus Status = iota
	Success
	Error
	RequestTimeout
)

// Res is an individual response sent by the Remote after receiving a Req.
// The response describes the type of the response data if it was successful (encoded as JSON),
// otherwise an error is returned
type Res struct {
	Status Status
	Type   string
	Data   any
	Error  error
}
