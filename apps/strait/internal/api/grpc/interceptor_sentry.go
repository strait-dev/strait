package grpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"strait/internal/telemetry"
)

type grpcSentryMetadata struct {
	edition string
	mode    string
	region  string
	version string
}

func unarySentryInterceptor(meta grpcSentryMetadata) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		ctx = ensureGRPCSentryHub(ctx)
		configureGRPCSentryScope(ctx, meta, map[telemetry.SentryTag]string{
			telemetry.TagService: grpcServiceName(info.FullMethod),
			telemetry.TagRPC:     grpcRPCName(info.FullMethod),
		})
		defer func() {
			if r := recover(); r != nil {
				captureGRPCSentryError(ctx, meta, info.FullMethod, grpcPanicError(r))
				panic(r)
			}
		}()
		resp, err = handler(ctx, req)
		addGRPCSentryBreadcrumb(ctx, info.FullMethod, false, err)
		captureGRPCSentryError(ctx, meta, info.FullMethod, err)
		return resp, err
	}
}

func streamSentryInterceptor(meta grpcSentryMetadata) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ensureGRPCSentryHub(ss.Context())
		configureGRPCSentryScope(ctx, meta, map[telemetry.SentryTag]string{
			telemetry.TagService: grpcServiceName(info.FullMethod),
			telemetry.TagRPC:     grpcRPCName(info.FullMethod),
		})
		wrapped := &sentryServerStream{ServerStream: ss, ctx: ctx}
		defer func() {
			if r := recover(); r != nil {
				captureGRPCSentryError(ctx, meta, info.FullMethod, grpcPanicError(r))
				panic(r)
			}
		}()
		err := handler(srv, wrapped)
		addGRPCSentryBreadcrumb(ctx, info.FullMethod, true, err)
		captureGRPCSentryError(ctx, meta, info.FullMethod, err)
		return err
	}
}

func addGRPCSentryBreadcrumb(ctx context.Context, method string, stream bool, err error) {
	message := "grpc unary"
	if stream {
		message = "grpc stream"
	}
	telemetry.AddSentryBreadcrumb(ctx, "grpc.server", message, map[string]any{
		"service":   grpcServiceName(method),
		"rpc":       grpcRPCName(method),
		"grpc_code": status.Code(err).String(),
	})
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

func configureGRPCSentryScope(ctx context.Context, meta grpcSentryMetadata, tags map[telemetry.SentryTag]string) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	hub.ConfigureScope(func(scope *sentry.Scope) {
		for k, v := range sentryGRPCContextTags(ctx, tags) {
			telemetry.SetSentryTag(scope, k, v)
		}
		if meta.hasRequiredTags() {
			for k, v := range telemetry.RequiredSentryTags(meta.edition, telemetry.SubsystemGRPC, meta.mode, meta.region, meta.version) {
				telemetry.SetSentryTag(scope, k, v)
			}
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
			"service":        tags[telemetry.TagService],
			"rpc":            tags[telemetry.TagRPC],
			"project_id":     grpcProjectIDFromContext(ctx),
			"api_key_id":     grpcAPIKeyIDFromContext(ctx),
			"worker_id":      tags[telemetry.TagWorkerID],
			"environment_id": grpcEnvironmentIDFromContext(ctx),
		})
	})
}

func captureGRPCSentryError(ctx context.Context, meta grpcSentryMetadata, method string, err error) {
	if err == nil || !shouldCaptureGRPCSentryError(err) {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	configureGRPCSentryScope(ctx, meta, map[telemetry.SentryTag]string{
		telemetry.TagService:  grpcServiceName(method),
		telemetry.TagRPC:      grpcRPCName(method),
		telemetry.TagGRPCCode: status.Code(err).String(),
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

func sentryGRPCContextTags(ctx context.Context, tags map[telemetry.SentryTag]string) map[telemetry.SentryTag]string {
	out := map[telemetry.SentryTag]string{}
	for k, v := range tags {
		if v != "" {
			out[k] = v
		}
	}
	if projectID := grpcProjectIDFromContext(ctx); projectID != "" {
		out[telemetry.TagProjectID] = projectID
	}
	if orgID := OrgIDFromContext(ctx); orgID != "" {
		out[telemetry.TagOrgID] = orgID
	}
	if apiKeyID := grpcAPIKeyIDFromContext(ctx); apiKeyID != "" {
		out[telemetry.TagAPIKeyID] = apiKeyID
		out[telemetry.TagActorID] = "apikey:" + apiKeyID
		out[telemetry.TagActorType] = "api_key"
	}
	if envID := grpcEnvironmentIDFromContext(ctx); envID != "" {
		out[telemetry.TagEnvironmentID] = envID
	}
	if traceID, spanID := grpcOTelTraceIDs(ctx); traceID != "" {
		out[telemetry.TagTraceID] = traceID
		if spanID != "" {
			out[telemetry.TagSpanID] = spanID
		}
	}
	return out
}

func configureGRPCSentryAPIKeyScope(ctx context.Context) {
	configureGRPCSentryScope(ctx, grpcSentryMetadata{}, map[telemetry.SentryTag]string{})
}

func configureGRPCSentryWorkerScope(ctx context.Context, workerID, name, hostname, sdkLanguage, sdkVersion string) {
	configureGRPCSentryScope(ctx, grpcSentryMetadata{}, map[telemetry.SentryTag]string{
		telemetry.TagWorkerID:    workerID,
		telemetry.TagWorkerName:  name,
		telemetry.TagWorkerHost:  hostname,
		telemetry.TagSDKLanguage: sdkLanguage,
		telemetry.TagSDKVersion:  sdkVersion,
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

func (m grpcSentryMetadata) hasRequiredTags() bool {
	return m.edition != "" || m.mode != "" || m.region != "" || m.version != ""
}

func grpcServiceName(fullMethod string) string {
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	service, _, ok := strings.Cut(fullMethod, "/")
	if !ok {
		return fullMethod
	}
	return service
}

func grpcRPCName(fullMethod string) string {
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	_, rpc, ok := strings.Cut(fullMethod, "/")
	if !ok {
		return fullMethod
	}
	return rpc
}
