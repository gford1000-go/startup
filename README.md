[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://en.wikipedia.org/wiki/MIT_License)
[![Documentation](https://img.shields.io/badge/Documentation-GoDoc-green.svg)](https://godoc.org/github.com/gford1000-go/startup)

# Startup

A simple mechanism to start goroutines in reliably, capturing interrupts and allowing a smooth shutdown of the application.

```go
func main() {
    myMain := func(ctx context.Context) {
        // Do some work, until context is Done
        <-ctx.Done()
    }

    StartFunctions(context.Background(), []StartableFunction{
        myMain,
    })
```

`StartFunctions` handles unrecovered `panic`s in the supplied functions (but not any goroutines that they launch), as well as interrupts.

Any number of `StartableFunction` can be provided to `StartFunctions`, with each running within their own goroutine and with their own, independent `context`.

## Options

* `WithLogging` enables logging behaviour, useful for debugging (default: no logging)
* `WithTimeout` allows a timeout to be specified for `StartFunctions` to exit (default: 30 seconds)

See examples for usage.
