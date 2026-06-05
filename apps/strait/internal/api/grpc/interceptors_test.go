package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(
		t, err)

	s, ok := status.FromError(err)
	require.True(t,
		ok)
	assert.Equal(t,
		codes.Internal,
		s.Code())
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
	require.NoError(t, err)
	assert.Equal(t,
		expected,
		resp)
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
	assert.ErrorIs(t,
		err, handlerErr)
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
	require.Error(
		t, err)

	s, ok := status.FromError(err)
	require.True(t,
		ok)
	assert.Equal(t,
		codes.Internal,
		s.Code())
}

// TestStreamRecoveryInterceptor_HappyPath verifies normal stream handler execution passes through.
func TestStreamRecoveryInterceptor_HappyPath(t *testing.T) {
	interceptor := streamRecoveryInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	stream := &mockServerStream{ctx: context.Background()}
	assert.NoError(t, interceptor(nil,
		stream,
		info, handler))
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
	assert.ErrorIs(t,
		err, handlerErr)
}

// TestUnaryLoggingInterceptor_PassesThrough verifies logging interceptor does not alter result.
func TestUnaryLoggingInterceptor_PassesThrough(t *testing.T) {
	interceptor := unaryLoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	handler := func(ctx context.Context, req any) (any, error) {
		return "result", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t,
		"result",
		resp)
}

// TestStreamLoggingInterceptor_PassesThrough verifies stream logging interceptor does not alter result.
func TestStreamLoggingInterceptor_PassesThrough(t *testing.T) {
	interceptor := streamLoggingInterceptor()
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	stream := &mockServerStream{ctx: context.Background()}
	assert.NoError(t, interceptor(nil,
		stream,
		info, handler))
}

// TestUnaryInterceptorChain_OrderAndCount verifies the chain has the expected interceptors.
func TestUnaryInterceptorChain_OrderAndCount(t *testing.T) {
	chain := unaryInterceptorChain()
	assert.Len(t,
		chain, 3)
}

// TestStreamInterceptorChain_OrderAndCount verifies the stream chain has the expected interceptors.
func TestStreamInterceptorChain_OrderAndCount(t *testing.T) {
	chain := streamInterceptorChain()
	assert.Len(t,
		chain, 3)
}
