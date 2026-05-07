package grpc

import (
	"context"
	"log/slog"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// unaryInterceptorChain returns the ordered list of unary server interceptors.
func unaryInterceptorChain() []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		unaryRecoveryInterceptor(),
		unarySentryInterceptor(),
		unaryLoggingInterceptor(),
	}
}

// streamInterceptorChain returns the ordered list of stream server interceptors.
func streamInterceptorChain() []grpc.StreamServerInterceptor {
	return []grpc.StreamServerInterceptor{
		streamRecoveryInterceptor(),
		streamSentryInterceptor(),
		streamLoggingInterceptor(),
	}
}

// unaryRecoveryInterceptor recovers from panics in unary handlers.
func unaryRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc unary panic recovered",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// streamRecoveryInterceptor recovers from panics in streaming handlers.
func streamRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc stream panic recovered",
					"method", info.FullMethod,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// unaryLoggingInterceptor logs unary RPC calls at debug level.
func unaryLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			slog.Debug("grpc unary call error", "method", info.FullMethod, "error", err)
		}
		return resp, err
	}
}

// streamLoggingInterceptor logs stream RPC connections at debug level.
func streamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		slog.Debug("grpc stream connected", "method", info.FullMethod)
		err := handler(srv, ss)
		if err != nil {
			slog.Debug("grpc stream closed with error", "method", info.FullMethod, "error", err)
		}
		return err
	}
}
