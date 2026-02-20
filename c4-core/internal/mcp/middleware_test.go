package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

// TestMiddlewareChain verifies that 3 middlewares execute in correct order.
func TestMiddlewareChain(t *testing.T) {
	reg := NewRegistry()

	var order []string

	// Middleware A (registered first = outermost)
	reg.Use(func(next HandlerFunc) HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			order = append(order, "A-before")
			result, err := next(args)
			order = append(order, "A-after")
			return result, err
		}
	})

	// Middleware B
	reg.Use(func(next HandlerFunc) HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			order = append(order, "B-before")
			result, err := next(args)
			order = append(order, "B-after")
			return result, err
		}
	})

	// Middleware C (registered last = innermost)
	reg.Use(func(next HandlerFunc) HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			order = append(order, "C-before")
			result, err := next(args)
			order = append(order, "C-after")
			return result, err
		}
	})

	reg.Register(ToolSchema{Name: "test_tool"}, func(args json.RawMessage) (any, error) {
		order = append(order, "handler")
		return "ok", nil
	})

	result, err := reg.Call("test_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %v, want ok", result)
	}

	// Expected order: A-before, B-before, C-before, handler, C-after, B-after, A-after
	expected := []string{"A-before", "B-before", "C-before", "handler", "C-after", "B-after", "A-after"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d; got %v", len(order), len(expected), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q; full order: %v", i, order[i], v, order)
		}
	}
}

// TestMiddlewareNoOp verifies that with zero middlewares, existing behavior is identical.
func TestMiddlewareNoOp(t *testing.T) {
	reg := NewRegistry()

	// No Use() calls — zero middlewares

	reg.Register(ToolSchema{Name: "plain_tool"}, func(args json.RawMessage) (any, error) {
		return "direct", nil
	})

	result, err := reg.Call("plain_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "direct" {
		t.Errorf("result = %v, want direct", result)
	}
}

// TestBlockingHandlerMiddleware verifies that BlockingHandlerFunc also passes through the chain.
func TestBlockingHandlerMiddleware(t *testing.T) {
	reg := NewRegistry()

	var middlewareCalled bool
	reg.Use(func(next HandlerFunc) HandlerFunc {
		return func(args json.RawMessage) (any, error) {
			middlewareCalled = true
			return next(args)
		}
	})

	reg.RegisterBlocking(ToolSchema{Name: "blocking_tool"}, func(ctx context.Context, args json.RawMessage) (any, error) {
		return "blocked", nil
	})

	result, err := reg.Call("blocking_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "blocked" {
		t.Errorf("result = %v, want blocked", result)
	}
	if !middlewareCalled {
		t.Error("middleware was not called for blocking handler")
	}
}
