package startup

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"time"
)

// FunctionOptions are options provided to StartableFunctions
type FunctionOptions struct {
	// DiscoveryService is provided if specified as an option for StartFunctions
	DiscoveryService DiscoveryService
}

// StartableFunction defines a func that can be provided to StartFunctions
type StartableFunction func(context.Context, *FunctionOptions)

// Options allow the behaviour of StartFunctions to be modified
type Options struct {
	// Logger specifies which log.Logger should be used (default is no logging)
	Logger *log.Logger
	// ReportPanicsOnly will limit logging to recording panics only, if set to true
	ReportPanicsOnly bool
	// Timeout specifies the duration to wait for StartableFunctions to gracefully exit
	Timeout time.Duration
	// DiscoveryService will create a new DiscoveryService that is provided to StartableFunctions
	DiscoveryService bool
}

// WithLogging allows a log.Logger to be specified for capturing StartFunctions activity.
// If no Logger is provided, then no logging will be performed.
// If unhandledPanicsOnly is set to true, then only unrecovered panics are logged, rather
// than all logging activity.  This makes it easier to see which StartableFunction failed.
func WithLogging(l *log.Logger, unhandledPanicsOnly bool) func(*Options) {
	return func(o *Options) {
		o.Logger = l
		o.ReportPanicsOnly = unhandledPanicsOnly
	}
}

// WithTimeout specifies the duration to allow for the StartableFunctions to exit gracefully.
// Default is 30 seconds.
func WithTimeout(d time.Duration) func(*Options) {
	return func(o *Options) {
		if d > 0 {
			o.Timeout = d
		}
	}
}

// WithDiscoveryService specifies a DiscoveryService should be created
func WithDiscoveryService() func(*Options) {
	return func(o *Options) {
		o.DiscoveryService = true
	}
}

var defaultOptions = Options{
	Timeout: 30 * time.Second,
}

// StartFunctions starts the specified StartableFunctions in separate goroutines, each with
// independent contexts.
// Should one of the functions exit, whether expected or due to a panic, then the contexts
// of the other functions will be completed, so they will be expected to detect this and
// shutdown gracefully as well.
// Standard interrupts (CTRL-C) are captured, and these will trigger a shutdown request to
// all functions.
func StartFunctions(ctx context.Context, fs []StartableFunction, opts ...func(*Options)) {

	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	pause := func() {
		<-time.After(time.Millisecond)
	}

	logger := func(s string) {
		if o.Logger != nil && !o.ReportPanicsOnly {
			o.Logger.Println(s)
		}
	}

	logPanic := func(err error) {
		if o.Logger != nil {
			o.Logger.Println(err)
		}
	}

	funcOps := FunctionOptions{}

	if o.DiscoveryService {
		funcOps.DiscoveryService = NewDiscoveryService()
	}

	cs := make([]context.Context, 0, len(fs))
	cfs := make([]context.CancelFunc, 0, len(fs))
	chs := make([]chan struct{}, 0, len(fs))
	for range len(fs) {
		// Contexts for the functions are independent of each other and the supplied context
		c, cf := context.WithCancel(context.Background())
		cs = append(cs, c)
		cfs = append(cfs, cf)
		chs = append(chs, make(chan struct{}, 1))
	}

	// This context is used to prevent this function from exiting
	// until a shutdown condition is met.
	exitCtx, exitCancel := context.WithCancel(context.Background())
	defer exitCancel()

	// Create a new cancellable context, which will handle graceful cancellation of the functions
	shutdownCtx, shutdownCancel := context.WithCancel(ctx)
	defer shutdownCancel()

	go func() {
		<-shutdownCtx.Done()

		logger("cancelling all contexts")
		for _, cf := range cfs {
			cf()
		}

		exitCancel() // Will now start waiting for shutdowns to complete
	}()

	// Trap interrupts
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Exits either when interrupt is detected, or when told to shutdown
	go func() {
		defer signal.Stop(signalChan)

		select {
		case <-signalChan:
			logger("received interrupt")
			shutdownCancel() // Trigger shutdowns
		case <-shutdownCtx.Done():
			// Requested to shutdown as well
		}
	}()

	pause()

	// Wrapper ensures graceful launch and shutdown, recovering from unhandled panics from functions
	// Note this doesn't deal with all unhandled panics: if functions start further goroutines
	// which then panic, that scenario is uncontrolled
	fWrapper := func(ctx context.Context, ctxCancel context.CancelFunc, ch chan struct{}, f StartableFunction) {

		inner := func(ctx context.Context, f StartableFunction) (err error) {
			defer ctxCancel() // Order ensures the supplied ctx is aways cancelled when f() exits
			defer func() {
				ch <- struct{}{}
			}()
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("caught unhandled panic in (%s): %v", runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name(), r)
				}
			}()

			f(ctx, &funcOps)
			return nil
		}

		go func() {
			defer shutdownCancel() // Always cancel the cancellable context, triggering shutdown

			err := inner(ctx, f)
			if err != nil {
				logPanic(err)
			}
		}()

		pause()
	}

	// Start the functions in their own goroutines
	for i, f := range fs {
		fWrapper(cs[i], cfs[i], chs[i], f)
	}

	// Handle shutdown with optional timeout
	ch := make(chan struct{})

	// In shutdown sequence, each function's inner() will push a struct{}{} to notify that it has exited
	// So exit will be signalled once all functions have exited
	go func() {
		defer func() {
			ch <- struct{}{}
		}()

		for _, c := range chs {
			<-c
		}
	}()

	pause()

	// Wait for notification to exit
	<-exitCtx.Done()

	logger("waiting for Done() from contexts")
	select {
	case <-ch:
		logger("all contexts are Done()")
	case <-time.After(o.Timeout):
		logger("timed out waiting for Done() from contexts")
	}
}
