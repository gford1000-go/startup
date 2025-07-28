package startup

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"testing"
	"time"
)

func Example() {

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	myMain := func(ctx context.Context, opts *FunctionOptions) {
		logger.Println("starting myMain")
		defer logger.Println("exiting myMain")

		// Emulates doing something
		<-time.After(50 * time.Millisecond)
	}

	StartFunctions(context.Background(), []StartableFunction{
		myMain,
	}, WithLogging(logger, true))

	fmt.Println(&buf)
	// Output:
	// starting myMain
	// exiting myMain
}

func ExampleStartFunctions() {

	mainBoom := func(ctx context.Context, opts *FunctionOptions) {
		<-time.After(50 * time.Millisecond)
		panic("Boom!")
	}

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	StartFunctions(context.Background(), []StartableFunction{
		mainBoom,
	}, WithLogging(logger, true))

	fmt.Println(&buf)
	// Output:
	// caught unhandled panic in (github.com/gford1000-go/startup.ExampleStartFunctions.func1): Boom!
}

func ExampleStartFunctions_second() {

	myMain := func(ctx context.Context, opts *FunctionOptions) {
		defer fmt.Println("myMain exited")

		// Emulate finishing work
		<-time.After(50 * time.Millisecond)
	}
	anotherFn := func(ctx context.Context, opts *FunctionOptions) {
		defer fmt.Println("anotherFn exited as well")

		// Not finished, but will exit
		<-ctx.Done()
	}

	StartFunctions(context.Background(), []StartableFunction{
		myMain,
		anotherFn,
	})

	// Output:
	// myMain exited
	// anotherFn exited as well
}

func ExampleStartFunctions_external_context_done() {

	myFunc := func(ctx context.Context, opts *FunctionOptions) {
		defer fmt.Println("myFunc exited")

		<-ctx.Done() // Busy until told to exit
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	StartFunctions(ctx, []StartableFunction{
		myFunc,
	})

	// Output:
	// myFunc exited
}

func TestStartFunctions(t *testing.T) {

	err := StartFunctions(context.Background(), nil)

	if err != ErrMissingStartableFunctions {
		t.Fatalf("Expected error: ErrMissingStartableFunctions, got: %v", err)
	}
}
