// package router_test to avoid import cycle
// TODO: modify existing dealer_test.go to work from the router_test package
package router_test

import (
	"context"
	"encoding/json"
	_ "errors"
	_ "fmt"
	"testing"
	"time"

	"github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/router"
	"github.com/gammazero/nexus/stdlog"
	"github.com/gammazero/nexus/transport/serialize"
	"github.com/gammazero/nexus/wamp"
)

var (
	debug  bool
	logger stdlog.StdLog
)

var clientRoles = wamp.Dict{
	"roles": wamp.Dict{
		"subscriber": wamp.Dict{
			"features": wamp.Dict{
				"publisher_identification": true,
			},
		},
		"publisher": wamp.Dict{
			"features": wamp.Dict{
				"subscriber_blackwhite_listing": true,
			},
		},
		"callee": wamp.Dict{},
		"caller": wamp.Dict{
			"features": wamp.Dict{
				"call_timeout": true,
			},
		},
	},
	"authmethods": wamp.List{"anonymous", "ticket"},
}

func newTestRouter() (router.Router, error) {
	config := &router.Config{
		RealmConfigs: []*router.RealmConfig{
			{
				URI:              wamp.URI("nexus.test.realm"),
				StrictURI:        false,
				AnonymousAuth:    true,
				AllowDisclose:    false,
				EnableMetaKill:   true,
				EnableMetaModify: true,
			},
		},
		Debug: debug,
	}
	return router.NewRouter(config, logger)
}

func newTestClient(r router.Router) (*client.Client, error) {
	cfg := client.Config{
		Realm:           "nexus.test.realm",
		ResponseTimeout: 500 * time.Millisecond,
		Logger:          logger,
		Debug:           false,
	}
	return client.ConnectLocal(r, cfg)
}

// --- Decorator Testing ---

func TestPreProcessDecoratorErrorResult(t *testing.T) {
	router, _ := newTestRouter()
	callee, _ := newTestClient(router)
	decoratee, _ := newTestClient(router)
	caller, _ := newTestClient(router)
	ctx := context.Background()

	// test pre-process decorator with error return.
	handlerFooBar := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		t.Fatal("Error: unexpected URI, should return Error!")
		return &client.InvokeResult{}
	}

	decoratorHandler := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		// handler returns an ERROR message: it should be sent to the caller.
		t.Logf("Returning error\n")
		return &client.InvokeResult{
			Err: wamp.URI("wamp.ErrCanceled"),
		}
	}
	// Register target URI (never used, since ERROR)
	if err := callee.Register("foo.test.bar.error", handlerFooBar, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Register handler URI
	if err := decoratee.Register("decoratortest.handlerURI.error", decoratorHandler, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Add preprocess decorator
	args := wamp.List{
		"preprocess",
		"foo.test.bar.error",
		"exact",
		"decoratortest.handlerURI.error",
		0,
		"sync"}
	if _, err := decoratee.Call(ctx, "wamp.decorator.add", nil, args, nil, ""); err != nil {
		t.Fatalf("Error calling wamp.decorator.add: %v\n", err)
	}

	// Trigger preprocess decorator
	rsp, err := caller.Call(ctx, "foo.test.bar.error", nil, nil, nil, "")
	if err == nil {
		// FIXME: this should return an error message to the caller. Instead it returns an error message nested into a response message.
		t.Errorf("Expected an ERROR, got %v\n", rsp)
	}
	t.Logf("Got error: %v\n", err)
}

func TestPreProcessDecoratorEmptyResult(t *testing.T) {
	router, _ := newTestRouter()
	callee, _ := newTestClient(router)
	decoratee, _ := newTestClient(router)
	caller, _ := newTestClient(router)
	ctx := context.Background()
	done := make(chan bool, 1)

	// test proprocess decorator: return nothing
	handlerFooBar := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		done <- true
		return &client.InvokeResult{}
	}

	decoratorHandler := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		t.Log("Running decoratorHandler")
		// returning nothing: the call should be processed as usual
		return &client.InvokeResult{}
		// return nil
	}

	// Register old target URI
	if err := callee.Register("foo.test.bar.nothing", handlerFooBar, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Register handler URI
	if err := decoratee.Register("decoratortest.handlerURI.nothing", decoratorHandler, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Add preprocess decorator
	args := wamp.List{
		"preprocess",
		"foo.test.bar.nothing",
		"exact",
		"decoratortest.handlerURI.nothing",
		0,
		"sync"}
	if _, err := decoratee.Call(ctx, "wamp.decorator.add", nil, args, nil, ""); err != nil {
		t.Fatalf("Error calling wamp.decorator.add: %v\n", err)
	}

	// Trigger preprocess decorator
	rsp, err := caller.Call(ctx, "foo.test.bar.nothing", nil, nil, nil, "")
	if err != nil {
		t.Fatalf("Error calling preprocess decorator: %v\n", err)
	}
	rspp, _ := json.MarshalIndent(rsp, "", "  ")
	t.Logf("rsp is: %s\n", rspp)
	go func() {
		time.Sleep(5 * time.Second)
		done <- false
	}()
	if !<-done {
		t.Error("Preprocess Decorator: HandlerFooBar not called after 5 seconds!")
	}
}

func TestPreProcessDecoratorRedirectResult(t *testing.T) {
	router, _ := newTestRouter()
	callee, _ := newTestClient(router)
	decoratee, _ := newTestClient(router)
	caller, _ := newTestClient(router)
	ctx := context.Background()
	done := make(chan bool, 1)

	handlerFooBar := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		t.Fatal("Error: unexpected URI, should return modified URI!")
		return &client.InvokeResult{}
	}

	handlerSuccess := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		t.Logf("Got to the rewritten handler, %v, %v, %v\n", args, kwargs, details)
		done <- true
		return &client.InvokeResult{}
	}

	decoratorHandler := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		returnCall := serialize.WampMessageToList(&wamp.Call{
			Request:     args[0].(*wamp.Call).Request,
			Options:     wamp.Dict{},
			Procedure:   wamp.URI("foo.rewrite.bar"),
			Arguments:   wamp.List{},
			ArgumentsKw: wamp.Dict{},
		})
		t.Log("running decoratorHandler")
		return &client.InvokeResult{
			// pre-process handler returns a new CALL message: it should be used instead of the original CALL message
			Args: wamp.List{returnCall},
		}
	}
	// Register old target URI
	if err := callee.Register("foo.test.bar.rewrite", handlerFooBar, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Register target URI
	if err := callee.Register("foo.rewrite.bar", handlerSuccess, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Register handler URI
	if err := decoratee.Register("decoratortest.handlerURI", decoratorHandler, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Add preprocess decorator
	args := wamp.List{
		"preprocess",
		"foo.test.bar.rewrite",
		"exact",
		"decoratortest.handlerURI",
		0,
		"sync"}

	if _, err := decoratee.Call(ctx, "wamp.decorator.add", nil, args, nil, ""); err != nil {
		t.Fatalf("Error calling wamp.decorator.add: %v\n", err)
	}

	// Trigger preprocess decorator
	rsp, err := caller.Call(ctx, "foo.test.bar.rewrite", nil, nil, nil, "")
	if err != nil {
		t.Fatalf("Error calling preprocess decorator: %v\n", err)
	}
	rspp, _ := json.MarshalIndent(rsp, "", "  ")
	t.Logf("rsp is: %s\n", rspp)
	go func() {
		time.Sleep(5 * time.Second)
		done <- false
	}()
	if !<-done {
		// FIXME: This should redirect the Call to the target URI "test.rewrite.URI". Instead it returns the new call to the caller, again as a nested call.
		t.Error("Preprocess Decorator: Call not redirected after 5 seconds!")
	}
}

func TestPreProcessDecoratorAsync(t *testing.T) {
	router, _ := newTestRouter()
	callee, _ := newTestClient(router)
	decoratee, _ := newTestClient(router)
	caller, _ := newTestClient(router)
	ctx := context.Background()
	done := make(chan bool, 1)

	// test proprocess decorator: return nothing
	handlerFooBar := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		done <- true
		return &client.InvokeResult{}
	}

	decoratorHandler := func(ctx context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
		t.Log("Running decoratorHandler")
		// returning nothing: the call should be processed as usual
		return &client.InvokeResult{}
		// return nil
	}

	// Register old target URI
	if err := callee.Register("foo.test.bar.nothing", handlerFooBar, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Register handler URI
	if err := decoratee.Register("decoratortest.handlerURI.nothing", decoratorHandler, nil); err != nil {
		t.Fatalf("failed to register procedure: %v\n", err)
	}

	// Add preprocess decorator
	args := wamp.List{
		"preprocess",
		"foo.test.bar.nothing",
		"exact",
		"decoratortest.handlerURI.nothing",
		0,
		"async"}
	if _, err := decoratee.Call(ctx, "wamp.decorator.add", nil, args, nil, ""); err != nil {
		t.Fatalf("Error calling wamp.decorator.add: %v\n", err)
	}

	// Trigger preprocess decorator
	rsp, err := caller.Call(ctx, "foo.test.bar.nothing", nil, nil, nil, "")
	if err != nil {
		t.Fatalf("Error calling preprocess decorator: %v\n", err)
	}
	rspp, _ := json.MarshalIndent(rsp, "", "  ")
	t.Logf("rsp is: %s\n", rspp)
	go func() {
		time.Sleep(5 * time.Second)
		done <- false
	}()
	if !<-done {
		t.Error("Preprocess Decorator: HandlerFooBar not called after 5 seconds!")
	}
}
