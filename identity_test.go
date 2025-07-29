package startup

import (
	"context"
	"fmt"
	"log"
	"time"
)

func ExampleCreateAndRegisterID() {

	// This handles all requests it receives by reflecting what it gets
	bobHandler := func(ctx context.Context, r1 *Req, r2 *Res) {
		r2.Type = r1.Type
		r2.Data = r1.Data
		r2.Status = Success
	}

	bobsProcessing := func(ctx context.Context, opts *FunctionOptions, args ...any) {
		// Since Bob is only listening and reflecting, there is nothing to do
		// in this case, except for the StartableFunction but wait for the context to be Donee.
		<-ctx.Done()
	}

	aliceProcessing := func(ctx context.Context, opts *FunctionOptions, args ...any) {

		// Details of Bob are passed as the first arg
		if len(args) != 1 {
			panic("unexpected args")
		}
		bobID, ok := args[0].(string)
		if !ok {
			panic(fmt.Sprintf("expected string arg, got: %v", args[0]))
		}

		// Since we have called StartNamedFunctions, the DiscoveryService exists and
		// Alice's identity has been registered with it. So has Bob's, so can start a connection
		alice := opts.Identity

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
		{Name: "Bob", Func: bobsProcessing, Args: nil, Handler: bobHandler},
		{Name: "Alice", Func: aliceProcessing, Args: []any{"Bob"}, RegisterWithDiscoveryService: true},
	},
		WithLogging(log.Default(), true),
		WithTimeout(5*time.Second))

	// Output: true
}
