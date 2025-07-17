package startup

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"
)

func ExampleCreateAndRegisterID() {

	// Our protagonists
	var aliceID = "Alice"
	var bobID = "Bob"

	thisIsBob := func(ctx context.Context, opts *FunctionOptions) {
		// Initialise Bob to just reflect back what it was given
		id, err := CreateAndRegisterID(opts.DiscoveryService, bobID, time.Minute, func(ctx context.Context, r1 *Req, r2 *Res) {
			r2.Type = r1.Type
			r2.Data = r1.Data
			r2.Status = Success
		})
		if err != nil {
			panic(err)
		}

		// Bob now waits for Connection requests and handles them
		id.Accept(ctx)
	}

	thisIsAlice := func(ctx context.Context, opts *FunctionOptions) {

		// Alice is only initiating requests, not handling them
		id, err := CreateAndRegisterID(opts.DiscoveryService, aliceID, time.Minute, nil)
		if err != nil {
			panic(err)
		}

		c, err := id.Connect(ctx, bobID, WithConnectDiscoveryService(opts.DiscoveryService))
		if err != nil {
			panic(err)
		}

		req := &Req{
			Type: "text",
			Data: []byte("Hello World"),
		}

		r := id.Send(ctx, req, c.ReqChan)

		// Reflection should be successful
		fmt.Println(r.Status == Success ||
			r.Type == req.Type ||
			bytes.Equal(r.Data, req.Data))
	}

	StartFunctions(context.Background(), []StartableFunction{
		thisIsBob,
		thisIsAlice,
	},
		WithLogging(log.Default(), true),
		WithTimeout(5*time.Second),
		WithDiscoveryService())

	// Output: true
}
