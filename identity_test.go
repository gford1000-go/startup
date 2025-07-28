package startup

import (
	"context"
	"fmt"
	"log"
	"time"
)

func ExampleCreateAndRegisterID() {

	bobsProcessing := func(ctx context.Context, opts *FunctionOptions, args ...any) {
		// Initialise Bob to just reflect back what it was given
		bob, err := CreateAndRegisterID(opts.DiscoveryService, opts.Self, time.Minute, func(ctx context.Context, r1 *Req, r2 *Res) {
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

	aliceProcessing := func(ctx context.Context, opts *FunctionOptions, args ...any) {

		// Alice is only initiating requests, not handling them
		alice, err := CreateAndRegisterID(opts.DiscoveryService, opts.Self, time.Minute, nil)
		if err != nil {
			panic(err)
		}

		// Details of Bob are passed as the first arg
		if len(args) != 1 {
			panic("unexpected args")
		}
		bobID, ok := args[0].(string)
		if !ok {
			panic(fmt.Sprintf("expected string arg, got: %v", args[0]))
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
		{"Bob", bobsProcessing, nil},
		{"Alice", aliceProcessing, []any{"Bob"}},
	},
		WithLogging(log.Default(), true),
		WithTimeout(5*time.Second),
		WithDiscoveryService())

	// Output: true
}
