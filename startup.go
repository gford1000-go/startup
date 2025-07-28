package startup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"sync"
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

// ErrMissingStartableFunctions is raised if no StartableFunctions are provided to StartFunctions
var ErrMissingStartableFunctions = errors.New("at least one StartableFunction must be provided")

// StartFunctions starts the specified StartableFunctions in separate goroutines, each with
// independent contexts.
// Should one of the functions exit, whether expected or due to a panic, then the contexts
// of the other functions will be completed, so they will be expected to detect this and
// shutdown gracefully as well.
// Standard interrupts (CTRL-C) are captured, and these will trigger a shutdown request to
// all functions.
func StartFunctions(ctx context.Context, fs []StartableFunction, opts ...func(*Options)) error {

	if len(fs) == 0 {
		return ErrMissingStartableFunctions
	}

	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	f := &funcMgr{
		ctx: ctx,
		o:   o,
		cs:  make([]context.Context, 0, len(fs)),
		cfs: make([]context.CancelFunc, 0, len(fs)),
		chs: make([]chan struct{}, 0, len(fs)),
	}

	if f.o.DiscoveryService {
		f.funcOps.DiscoveryService = NewDiscoveryService()
	}

	// This context is used to prevent this function from exiting
	// until a shutdown condition is met.
	f.exitCtx, f.exitCancel = context.WithCancel(context.Background())

	// Create a new cancellable context, which will handle graceful cancellation of the functions
	f.shutdownCtx, f.shutdownCancel = context.WithCancel(ctx)

	// Start awaiting on shutdown requests
	f.startAwaitShutdown()

	// Capture interupts that could trigger shutdown
	f.startInterruptHandling()

	// Start the functions
	for _, fn := range fs {
		f.addFn(fn)
	}

	f.awaitExit()

	return nil
}

type funcMgr struct {
	ctx            context.Context
	o              Options
	funcOps        FunctionOptions
	lck            sync.Mutex
	cs             []context.Context
	cfs            []context.CancelFunc
	chs            []chan struct{}
	exitCtx        context.Context
	exitCancel     context.CancelFunc
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

func (f *funcMgr) exit() {
	f.exitCancel()
}

func (f *funcMgr) shutdown() {
	f.shutdownCancel()
}

func (f *funcMgr) startAwaitShutdown() {

	go func() {
		<-f.shutdownCtx.Done()

		f.logger("cancelling all contexts")

		// Gain lock as there is the possibility that addFn() could be
		// concurrently adding a futher StartableFunction
		f.lck.Lock()
		defer f.lck.Unlock()

		for _, cf := range f.cfs {
			cf()
		}

		f.exit() // Will now start waiting for shutdowns to complete
	}()

	f.pause()
}

func (f *funcMgr) startInterruptHandling() {
	// Trap interrupts
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Exits either when interrupt is detected, or when told to shutdown
	go func() {
		defer signal.Stop(signalChan)

		select {
		case <-signalChan:
			f.logger("received interrupt")
			f.shutdown() // Trigger shutdowns
		case <-f.shutdownCtx.Done():
			// Requested to shutdown as well
		}
	}()

	f.pause()
}

// Wrapper ensures graceful launch and shutdown, recovering from unhandled panics from functions
// Note this doesn't deal with all unhandled panics: if functions start further goroutines
// which then panic, that scenario is uncontrolled
func (f *funcMgr) fWrapper(ctx context.Context, ctxCancel context.CancelFunc, ch chan struct{}, sf StartableFunction) {

	inner := func(ctx context.Context, fn StartableFunction) (err error) {
		defer ctxCancel() // Order ensures the supplied ctx is aways cancelled when f() exits
		defer func() {
			ch <- struct{}{}
		}()
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("caught unhandled panic in (%s): %v", runtime.FuncForPC(reflect.ValueOf(sf).Pointer()).Name(), r)
			}
		}()

		fn(ctx, &f.funcOps)
		return nil
	}

	go func() {
		defer f.shutdown() // Always cancel the cancellable context, triggering shutdown

		err := inner(ctx, sf)
		if err != nil {
			f.logPanic(err)
		}
	}()

	f.pause()
}

// addFn creates and stores the scaffolding (contexts, chans etc.) needed to manage
// the lifetime of the provided StartableFunction, ensuring that it can close
// gracefully if it or another StartableFunction exits
func (f *funcMgr) addFn(fn StartableFunction) {

	f.lck.Lock()
	defer f.lck.Unlock()

	// Once lock obtained, only continue if shutdown context is not Done
	select {
	case <-f.shutdownCtx.Done():
	default:
		c, cf := context.WithCancel(context.Background())

		ch := make(chan struct{}, 1)
		f.cs = append(f.cs, c)
		f.cfs = append(f.cfs, cf)
		f.chs = append(f.chs, ch)

		f.fWrapper(c, cf, ch, fn)
	}
}

// awaitExit allows for graceful shutdown with optional timeout
func (f *funcMgr) awaitExit() {
	ch := make(chan struct{})
	defer close(ch)

	// Wait for notification to exit
	f.logger("waiting for Done() from exit context")
	<-f.exitCtx.Done()
	f.logger("received Done() for exit context")

	// In shutdown sequence, each function's inner() will push a struct{}{} to notify that it has exited
	// So exit will be signalled once all functions have exited
	go func() {
		defer func() {
			ch <- struct{}{}
		}()

		f.lck.Lock()
		defer f.lck.Unlock()

		for _, c := range f.chs {
			<-c
		}
	}()

	f.pause()

	f.logger("waiting for Done() from StartableFunction contexts")
	select {
	case <-ch:
		f.logger("all contexts are Done()")
	case <-time.After(f.o.Timeout):
		f.logger("timed out waiting for Done() from contexts")
	}

}

func (f *funcMgr) pause() {
	<-time.After(time.Millisecond)
}

func (f *funcMgr) logger(s string) {
	if f.o.Logger != nil && !f.o.ReportPanicsOnly {
		f.o.Logger.Println(s)
	}
}

func (f *funcMgr) logPanic(err error) {
	if f.o.Logger != nil {
		f.o.Logger.Println(err)
	}
}
