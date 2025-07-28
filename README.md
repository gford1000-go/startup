[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://en.wikipedia.org/wiki/MIT_License)
[![Documentation](https://img.shields.io/badge/Documentation-GoDoc-green.svg)](https://godoc.org/github.com/gford1000-go/startup)

# Startup

A simple mechanism to start goroutines reliably, capturing interrupts and allowing a smooth shutdown of the application.

There are two forms: `StartFunctions` and `StartNamedFunctions`

## StartFunctions

`StartFunctions` will launch the `StartableFunctions` anonymously.  In reality they will have a name assigned that is randomly generated, available from `opts.Self`.  `args` will always be empty for this form.

```go
func main() {
    myMain := func(ctx context.Context, opts FunctionOptions, args ...any) {
        // Do some work, until context is Done
        <-ctx.Done()
    }

    StartFunctions(context.Background(), []StartableFunction{
        myMain,
    })
```

`StartFunctions` handles unrecovered `panic`s in the supplied functions (but not any goroutines that they launch), as well as interrupts.

Any number of `StartableFunction` can be provided to `StartFunctions`, with each running within their own goroutine and with their own, independent `context`.

### Options

* `WithLogging` enables logging behaviour, useful for debugging (default: no logging)
* `WithTimeout` allows a timeout to be specified for `StartFunctions` to exit (default: 30 seconds)
* `WithDiscoveryService` creates a `DiscoveryService` so that functions can discover and communicate with each other

## StartNamedFunctions

`StartNamedFunctions` receives a slice of `FunctionDeclaration`s, which in addition to the `StartableFunction` may also optionally contain a unique name (obtainable from `opts.Self`) and a slice of `any` which will be passed as the `args` when the `StartableFunction` is executed.

```go
func main() {
    myFuncName := "FooBar"

    myMain := func(ctx context.Context, opts *FunctionOptions, args ...any) {
        if fmt.Sprintf("%s: %s", opts.Self, args[0]) != "FooBar: Hello World" {
            panic("unexpected behaviour")
        }

        <-ctx.Done()
    }

    StartNamedFunctions(context.Background(), []FunctionDeclaration{
        {Name: myFuncName, Func: myMain, Args: []any{"Hello World"}},
    })
```

`StartNamedFunctions` takes the same `Options` as `StartFunctions`.

See examples for usage.
