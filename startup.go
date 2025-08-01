package startup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// Self returns the name of this StartableFunction
	Self string
	// DiscoveryService is available only when using StartNamedFunctions
	DiscoveryService DiscoveryService
	// Identity is populated if the StartableFunction has been registered with the DiscoveryService
	Identity Identity
}

// StartableFunction defines a func that can be provided to StartFunctions
type StartableFunction func(context.Context, *FunctionOptions, ...any)

// FunctionDeclaration provides details about each StartableFunction
type FunctionDeclaration struct {
	// Name must be unique, if specified
	Name string
	// Func is the StartableFunction to be started, and must not be nil
	Func StartableFunction
	// Args will be passed to the StartableFunction as it is launched
	Args []any
	// RegisterWithDiscoveryService, if true, will attempt to register the StartableFunction with the
	// DiscoveryService,regardless of the value of Handler.  This allows outgoing connections
	// without requiring a listener to be established.
	// If false, the StartableFunction still has access to the DiscoveryService to register itself manually.
	RegisterWithDiscoveryService bool
	// Handler will be used to listen for and process incoming messages.
	// If nil, the StartableFunction still has access to the DiscoveryService to initate listening manually.
	Handler Handler
}

// createNameIfMissing ensures name is only set if it doesn't already exist
func createNameIfMissing(name string) string {
	if len(name) == 0 {
		b := make([]byte, 16)
		rand.Reader.Read(b)
		return hex.EncodeToString(b)
	}
	return name
}

// createArgsIfMissing ensures args slice is always populated
func createArgsIfMissing(args []any) []any {
	if args == nil {
		return []any{}
	}
	return args
}

// ErrNameAlreadyExists is raised if the specified Name for the StartableFunction is already in use
var ErrNameAlreadyExists = errors.New("function names must be unique")

// ErrFuncMustNotBeNil is raised when no StartableFunction has been assigned to the FunctionDeclaration
var ErrFuncMustNotBeNil = errors.New("function must not be nil")

func (f FunctionDeclaration) validate(m map[string]bool) error {
	// Name must be unique
	if _, ok := m[f.Name]; ok {
		return ErrNameAlreadyExists
	} else {
		m[f.Name] = true
	}

	if f.Func == nil {
		return ErrFuncMustNotBeNil
	}

	return nil
}

// Options allow the behaviour of StartFunctions to be modified
type Options struct {
	// Logger specifies which log.Logger should be used (default is no logging)
	Logger *log.Logger
	// ReportPanicsOnly will limit logging to recording panics only, if set to true
	ReportPanicsOnly bool
	// Timeout specifies the duration to wait for StartableFunctions to gracefully exit
	Timeout time.Duration
	// noDiscoveryService is not directly settable, set by StartFunctions
	noDiscoveryService bool
	// PauseDuration is the duration a routine will wait, to allow a goroutine it has started time to be to scheduled
	PauseDuration time.Duration
}

// OptionSetter type allows Options to be optionally set by caller to StartFunctions
type OptionSetter func(*Options) error

// WithLogging allows a log.Logger to be specified for capturing StartFunctions activity.
// If no Logger is provided, then no logging will be performed.
// If unhandledPanicsOnly is set to true, then only unrecovered panics are logged, rather
// than all logging activity.  This makes it easier to see which StartableFunction failed.
func WithLogging(l *log.Logger, unhandledPanicsOnly bool) OptionSetter {
	return func(o *Options) error {
		o.Logger = l
		o.ReportPanicsOnly = unhandledPanicsOnly
		return nil
	}
}

// ErrInvalidTimeout raised if WithTimeout() is called with a 0 or negative duration
var ErrInvalidTimeout = errors.New("exit timeout must be greater than zero")

// WithTimeout specifies the duration to allow for the StartableFunctions to exit gracefully.
// Default is 30 seconds.
func WithTimeout(d time.Duration) OptionSetter {
	return func(o *Options) error {
		if d > 0 {
			o.Timeout = d
			return nil
		}
		return ErrInvalidTimeout
	}
}

// withoutDiscoveryService specifies a DiscoveryService should NOT be created
// This is specified when StartFunctions is used rather than StartNamedFunctions,
// as the goroutines started by StartFunctions are anonymous, and hence no communication
// between them should be possible.
func withoutDiscoveryService() OptionSetter {
	return func(o *Options) error {
		o.noDiscoveryService = true
		return nil
	}
}

// ErrInvalidPauseTimeout raised if WithPauseDuration is less than one millisecond
var ErrInvalidPauseTimeout = errors.New("pause duration must be greater than one millisecond")

// WithPauseDuration specifies the pause for goroutine scheduling, must be greater than 1ms
func WithPauseDuration(d time.Duration) OptionSetter {
	return func(o *Options) error {
		if d > 1*time.Millisecond {
			o.PauseDuration = d
			return nil
		}
		return ErrInvalidPauseTimeout
	}
}

var defaultOptions = Options{
	Timeout:       30 * time.Second,
	PauseDuration: 1 * time.Millisecond,
}

// ErrMissingStartableFunctions is raised if no StartableFunctions are provided to StartFunctions
var ErrMissingStartableFunctions = errors.New("at least one StartableFunction must be provided")

// StartFunctions starts the StartableFunctions defined in the FunctionDeclarations
// in separate goroutines, each with independent contexts.
// Should one of the functions exit, whether expected or due to a panic, then the contexts
// of the other functions will be completed, so they will be expected to detect this and
// shutdown gracefully as well.
// Standard interrupts (CTRL-C) are captured, and these will trigger a shutdown request to
// all functions.
func StartFunctions(ctx context.Context, funcs []StartableFunction, opts ...OptionSetter) error {
	var dfs = []FunctionDeclaration{}
	for _, fn := range funcs {
		dfs = append(dfs, FunctionDeclaration{
			Func: fn,
		})
	}
	optsEx := append([]OptionSetter{}, opts...)
	optsEx = append(optsEx, withoutDiscoveryService())

	return StartNamedFunctions(ctx, dfs, optsEx...)
}

// StartNamedFunctions starts the StartableFunctions defined in the FunctionDeclarations
// in separate goroutines, each with independent contexts.
// Each FunctionDeclaration must either have no name specified (a unique one is assigned) or
// a unique to any other specified in the FunctionDeclaration list.  This allows them to communicate
// to each other via thew DiscoveryService, if that is optionally specified.
// Should one of the functions exit, whether expected or due to a panic, then the contexts
// of the other functions will be completed, so they will be expected to detect this and
// shutdown gracefully as well.
// Standard interrupts (CTRL-C) are captured, and these will trigger a shutdown request to
// all functions.
func StartNamedFunctions(ctx context.Context, funcs []FunctionDeclaration, opts ...OptionSetter) error {
	if len(funcs) == 0 {
		return ErrMissingStartableFunctions
	}

	names := make(map[string]bool, len(funcs))

	// Ensure name applied
	var myFuncs []FunctionDeclaration
	for _, fn := range funcs {
		myFuncs = append(myFuncs, FunctionDeclaration{
			Args:                         createArgsIfMissing(fn.Args),
			Func:                         fn.Func,
			Name:                         createNameIfMissing(fn.Name),
			Handler:                      fn.Handler,
			RegisterWithDiscoveryService: fn.RegisterWithDiscoveryService,
		})
	}

	// Validate
	for _, fn := range myFuncs {
		if err := fn.validate(names); err != nil {
			return err
		}
	}

	o := defaultOptions
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return err
		}
	}

	f := &funcMgr{
		ctx: ctx,
		o:   o,
		cs:  make([]context.Context, 0, len(myFuncs)),
		cfs: make([]context.CancelFunc, 0, len(myFuncs)),
		chs: make([]chan struct{}, 0, len(myFuncs)),
	}

	if !f.o.noDiscoveryService {
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
	for _, fn := range myFuncs {
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
func (f *funcMgr) fWrapper(ctx context.Context, ctxCancel context.CancelFunc, ch chan struct{}, fn FunctionDeclaration) {

	inner := func(ctx context.Context, fn *FunctionDeclaration) (err error) {
		defer ctxCancel() // Order ensures the supplied ctx is aways cancelled when fn.Func() exits
		defer func() {
			ch <- struct{}{}
		}()
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("caught unhandled panic in (%s): %v", runtime.FuncForPC(reflect.ValueOf(fn.Func).Pointer()).Name(), r)
			}
		}()

		// Set up funcOps specific to this StartableFunction, from defaults
		var funcOps = f.funcOps
		funcOps.Self = fn.Name

		// If DiscoveryService is running then can register the StartableFunction if requested
		// either directly via the RegisterWithDiscoveryService flag, or indirectly by the
		// presence of a Handler
		if funcOps.DiscoveryService != nil {
			if fn.RegisterWithDiscoveryService || fn.Handler != nil {
				identity, err := CreateAndRegisterID(funcOps.DiscoveryService, funcOps.Self, time.Minute, fn.Handler)
				if err != nil {
					return err
				}
				funcOps.Identity = identity
			}

			// Wait for Connection requests and handle them, until context is Done
			if fn.Handler != nil {
				go func(ctx context.Context, identity Identity) {
					defer f.logger(fmt.Sprintf("listening ended for %s", identity.ID()))

					f.logger(fmt.Sprintf("listening started for %s", identity.ID()))
					identity.Accept(ctx)
				}(ctx, funcOps.Identity)

				f.pause()
			}
		}

		f.logger(fmt.Sprintf("executing StartableFunction %s", fn.Name))
		defer f.logger(fmt.Sprintf("exited StartableFunction %s", fn.Name))

		fn.Func(ctx, &funcOps, fn.Args...)
		return nil
	}

	go func() {
		defer f.shutdown() // Always cancel the cancellable context, triggering shutdown

		err := inner(ctx, &fn)
		if err != nil {
			f.logPanic(err)
		}
	}()

	f.pause()
}

// addFn creates and stores the scaffolding (contexts, chans etc.) needed to manage
// the lifetime of the provided StartableFunction, ensuring that it can close
// gracefully if it or another StartableFunction exits
func (f *funcMgr) addFn(fn FunctionDeclaration) {

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

// awaitExit allows for graceful shutdown with optional timeout.
// There are only possible two scenarios:
// - A StartableFunction has exited, triggering shutdown of any/all others
// - The external context is Done(), which means all StartableFunctions should shut down
func (f *funcMgr) awaitExit() {

	select {
	case <-f.exitCtx.Done():
		// Wait for notification to exit
		f.logger("received Done() for exit context")
	case <-f.ctx.Done():
		// External context is Done, so attempt close down
		f.logger("received Done() for external context")
		f.shutdown()
	}

	ch := make(chan struct{})
	defer close(ch)

	// In shutdown sequence, each function's inner() will push a struct{}{} to notify that it has exited
	// So exit will be signalled to ch, only once all functions have exited
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
	<-time.After(f.o.PauseDuration)
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
