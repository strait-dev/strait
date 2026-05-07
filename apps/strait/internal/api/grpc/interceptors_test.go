package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestUnaryRecoveryInterceptor_Panic verifies that a panicking handler returns Internal status.
func TestUnaryRecoveryInterceptor_Panic(t *testing.T) {
	interceptor := unaryRecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	handler := func(ctx context.Context, req any) (any, error) {
		panic("intentional panic in test")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if err == nil {
		t.Fatal("expected error from panic recovery, got nil")
	}

	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %T %v", err, err)
	}
	if s.Code() != codes.Internal {
		t.Errorf("expected Internal status code, got %s", s.Code())
	}
}

// TestUnaryRecoveryInterceptor_HappyPath verifies normal handler execution passes through.
func TestUnaryRecoveryInterceptor_HappyPath(t *testing.T) {
	interceptor := unaryRecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	expected := "response-value"
	handler := func(ctx context.Context, req any) (any, error) {
		return expected, nil
	}

	resp, err := interceptor(context.Background(), "req", info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != expected {
		t.Errorf("expected %q, got %v", expected, resp)
	}
}

// TestUnaryRecoveryInterceptor_ErrorPassthrough verifies that handler errors are returned as-is.
func TestUnaryRecoveryInterceptor_ErrorPassthrough(t *testing.T) {
	interceptor := unaryRecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	handlerErr := errors.New("handler error")
	handler := func(ctx context.Context, req any) (any, error) {
		return nil, handlerErr
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if !errors.Is(err, handlerErr) {
		t.Errorf("expected handler error to pass through, got: %v", err)
	}
}

// mockServerStream is a minimal implementation of grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

// TestStreamRecoveryInterceptor_Panic verifies that a panicking stream handler returns Internal status.
func TestStreamRecoveryInterceptor_Panic(t *testing.T) {
	interceptor := streamRecoveryInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	handler := func(srv any, stream grpc.ServerStream) error {
		panic("intentional stream panic")
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := interceptor(nil, stream, info, handler)
	if err == nil {
		t.Fatal("expected error from stream panic recovery, got nil")
	}

	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %T %v", err, err)
	}
	if s.Code() != codes.Internal {
		t.Errorf("expected Internal status code, got %s", s.Code())
	}
}

// TestStreamRecoveryInterceptor_HappyPath verifies normal stream handler execution passes through.
func TestStreamRecoveryInterceptor_HappyPath(t *testing.T) {
	interceptor := streamRecoveryInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	stream := &mockServerStream{ctx: context.Background()}
	if err := interceptor(nil, stream, info, handler); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStreamRecoveryInterceptor_ErrorPassthrough verifies handler errors are returned.
func TestStreamRecoveryInterceptor_ErrorPassthrough(t *testing.T) {
	interceptor := streamRecoveryInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	handlerErr := errors.New("stream handler error")
	handler := func(srv any, stream grpc.ServerStream) error {
		return handlerErr
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := interceptor(nil, stream, info, handler)
	if !errors.Is(err, handlerErr) {
		t.Errorf("expected handler error to pass through, got: %v", err)
	}
}

// TestUnaryLoggingInterceptor_PassesThrough verifies logging interceptor does not alter result.
func TestUnaryLoggingInterceptor_PassesThrough(t *testing.T) {
	interceptor := unaryLoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	handler := func(ctx context.Context, req any) (any, error) {
		return "result", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "result" {
		t.Errorf("expected 'result', got %v", resp)
	}
}

// TestStreamLoggingInterceptor_PassesThrough verifies stream logging interceptor does not alter result.
func TestStreamLoggingInterceptor_PassesThrough(t *testing.T) {
	interceptor := streamLoggingInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	stream := &mockServerStream{ctx: context.Background()}
	if err := interceptor(nil, stream, info, handler); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestUnaryInterceptorChain_OrderAndCount verifies the chain has the expected interceptors.
func TestUnaryInterceptorChain_OrderAndCount(t *testing.T) {
	chain := unaryInterceptorChain()
	if len(chain) != 2 {
		t.Errorf("expected 2 interceptors in unary chain, got %d", len(chain))
	}
}

// TestStreamInterceptorChain_OrderAndCount verifies the stream chain has the expected interceptors.
func TestStreamInterceptorChain_OrderAndCount(t *testing.T) {
	chain := streamInterceptorChain()
	if len(chain) != 2 {
		t.Errorf("expected 2 interceptors in stream chain, got %d", len(chain))
	}
}
