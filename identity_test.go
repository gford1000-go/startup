package startup

import (
	"context"
	"fmt"
	"log"
	"time"
)

func ExampleCreateAndRegisterID() {

	// Our protagonists
	var aliceID = "Alice"
	var bobID = "Bob"

	bobsProcessing := func(ctx context.Context, opts *FunctionOptions) {
		// Initialise Bob to just reflect back what it was given
		bob, err := CreateAndRegisterID(opts.DiscoveryService, bobID, time.Minute, func(ctx context.Context, r1 *Req, r2 *Res) {
			r2.Type = r1.Type
			r2.Data = r1.Data
			r2.Status = Success
		})
		if err != nil {
			panic(err)
		}

		// Bob now waits for Connection requests and handles them
		bob.Accept(ctx)
	}

	aliceProcessing := func(ctx context.Context, opts *FunctionOptions) {

		// Alice is only initiating requests, not handling them
		alice, err := CreateAndRegisterID(opts.DiscoveryService, aliceID, time.Minute, nil)
		if err != nil {
			panic(err)
		}

		c, err := alice.Connect(ctx, bobID, WithConnectDiscoveryService(opts.DiscoveryService))
		if err != nil {
			panic(err)
		}

		req := &Req{
			Type: "text",
			Data: "Hello World",
		}

		r := alice.Send(ctx, req, c.ReqChan)

		// Reflection should be successful
		fmt.Println(r.Status == Success ||
			r.Type == req.Type ||
			r.Data.(string) == req.Data.(string))
	}

	StartNamedFunctions(context.Background(), []FunctionDeclaration{
		{bobID, bobsProcessing},
		{aliceID, aliceProcessing},
	},
		WithLogging(log.Default(), true),
		WithTimeout(5*time.Second),
		WithDiscoveryService())

	// Output: true
}
