package main

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestHandleSecondInstanceLaunchBringsWindowToFront(t *testing.T) {
	var gotShow bool
	var gotUnmin bool
	oldShow := windowShowFn
	oldUnmin := windowUnminimiseFn
	windowShowFn = func(ctx context.Context) {
		gotShow = true
	}
	windowUnminimiseFn = func(ctx context.Context) {
		gotUnmin = true
	}
	defer func() {
		windowShowFn = oldShow
		windowUnminimiseFn = oldUnmin
	}()

	a := &App{ctx: context.Background()}
	a.handleSecondInstanceLaunch(options.SecondInstanceData{})

	if !gotShow || !gotUnmin {
		t.Fatalf("show=%v unmin=%v", gotShow, gotUnmin)
	}
}

func TestHandleSecondInstanceLaunchIgnoresNilContext(t *testing.T) {
	var called bool
	oldShow := windowShowFn
	oldUnmin := windowUnminimiseFn
	windowShowFn = func(ctx context.Context) {
		called = true
	}
	windowUnminimiseFn = func(ctx context.Context) {
		called = true
	}
	defer func() {
		windowShowFn = oldShow
		windowUnminimiseFn = oldUnmin
	}()

	(&App{}).handleSecondInstanceLaunch(options.SecondInstanceData{})

	if called {
		t.Fatal("expected no window actions when context is nil")
	}
}
