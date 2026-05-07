package grpc

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func unarySentryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		ctx = ensureGRPCSentryHub(ctx)
		configureGRPCSentryScope(ctx, map[string]string{
			"grpc_method": info.FullMethod,
			"rpc_system":  "grpc",
		})
		defer func() {
			if r := recover(); r != nil {
				captureGRPCSentryError(ctx, info.FullMethod, grpcPanicError(r))
				panic(r)
			}
		}()
		resp, err = handler(ctx, req)
		captureGRPCSentryError(ctx, info.FullMethod, err)
		return resp, err
	}
}

func streamSentryInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ensureGRPCSentryHub(ss.Context())
		configureGRPCSentryScope(ctx, map[string]string{
			"grpc_method": info.FullMethod,
			"rpc_system":  "grpc",
		})
		wrapped := &sentryServerStream{ServerStream: ss, ctx: ctx}
		defer func() {
			if r := recover(); r != nil {
				captureGRPCSentryError(ctx, info.FullMethod, grpcPanicError(r))
				panic(r)
			}
		}()
		err := handler(srv, wrapped)
		captureGRPCSentryError(ctx, info.FullMethod, err)
		return err
	}
}

type sentryServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *sentryServerStream) Context() context.Context {
	return s.ctx
}

func ensureGRPCSentryHub(ctx context.Context) context.Context {
	if sentry.GetHubFromContext(ctx) != nil {
		return ctx
	}
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub().Clone())
}

func configureGRPCSentryScope(ctx context.Context, tags map[string]string) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	hub.ConfigureScope(func(scope *sentry.Scope) {
		for k, v := range sentryGRPCContextTags(ctx, tags) {
			scope.SetTag(k, v)
		}
		if apiKeyID := grpcAPIKeyIDFromContext(ctx); apiKeyID != "" {
			scope.SetUser(sentry.User{
				ID: "apikey:" + apiKeyID,
				Data: map[string]string{
					"actor_type": "api_key",
					"project_id": grpcProjectIDFromContext(ctx),
				},
			})
		}
		scope.SetContext("grpc.request", sentry.Context{
			"method":         tags["grpc_method"],
			"project_id":     grpcProjectIDFromContext(ctx),
			"api_key_id":     grpcAPIKeyIDFromContext(ctx),
			"worker_id":      tags["worker_id"],
			"environment_id": grpcEnvironmentIDFromContext(ctx),
		})
	})
}

func captureGRPCSentryError(ctx context.Context, method string, err error) {
	if err == nil || !shouldCaptureGRPCSentryError(err) {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	configureGRPCSentryScope(ctx, map[string]string{
		"grpc_method": method,
		"rpc_system":  "grpc",
		"grpc_code":   status.Code(err).String(),
	})
	hub.CaptureException(err)
}

func shouldCaptureGRPCSentryError(err error) bool {
	switch status.Code(err) {
	case codes.Internal, codes.Unknown, codes.DataLoss, codes.Unavailable:
		return true
	default:
		return false
	}
}

func sentryGRPCContextTags(ctx context.Context, tags map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range tags {
		if v != "" {
			out[k] = v
		}
	}
	if projectID := grpcProjectIDFromContext(ctx); projectID != "" {
		out["project_id"] = projectID
	}
	if orgID := OrgIDFromContext(ctx); orgID != "" {
		out["org_id"] = orgID
	}
	if apiKeyID := grpcAPIKeyIDFromContext(ctx); apiKeyID != "" {
		out["api_key_id"] = apiKeyID
		out["actor_id"] = "apikey:" + apiKeyID
		out["actor_type"] = "api_key"
	}
	if envID := grpcEnvironmentIDFromContext(ctx); envID != "" {
		out["environment_id"] = envID
	}
	if traceID, spanID := grpcOTelTraceIDs(ctx); traceID != "" {
		out["trace_id"] = traceID
		if spanID != "" {
			out["span_id"] = spanID
		}
	}
	return out
}

func configureGRPCSentryAPIKeyScope(ctx context.Context) {
	configureGRPCSentryScope(ctx, map[string]string{
		"rpc_system": "grpc",
	})
}

func configureGRPCSentryWorkerScope(ctx context.Context, workerID, name, hostname, sdkLanguage, sdkVersion string) {
	configureGRPCSentryScope(ctx, map[string]string{
		"rpc_system":   "grpc",
		"worker_id":    workerID,
		"worker_name":  name,
		"worker_host":  hostname,
		"sdk_language": sdkLanguage,
		"sdk_version":  sdkVersion,
	})
}

func grpcProjectIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(grpcCtxProjectIDKey).(string)
	return v
}

func grpcAPIKeyIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(grpcCtxAPIKeyIDKey).(string)
	return v
}

func grpcEnvironmentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(grpcCtxEnvironmentIDKey).(string)
	return v
}

func grpcOTelTraceIDs(ctx context.Context) (string, string) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "", ""
	}
	traceID := sc.TraceID()
	if !traceID.IsValid() {
		return "", ""
	}
	spanID := sc.SpanID()
	if !spanID.IsValid() {
		return traceID.String(), ""
	}
	return traceID.String(), spanID.String()
}

func grpcPanicError(panicValue any) error {
	return status.Error(codes.Internal, fmt.Sprintf("panic: %v", panicValue))
}
