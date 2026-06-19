package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Resource bounds for incoming worker messages. Without these, a malicious or
// buggy worker can register with millions of queues / in-flight tasks /
// log lines and exhaust server memory or DB capacity.
const (
	maxWorkerIDLen       = 128
	maxQueuesPerWorker   = 64
	maxQueueNameBytes    = 128
	maxJobSlugsPerWorker = 256
	maxJobSlugBytes      = 128
	maxInFlightTasks     = 256
	maxLogMessageBytes   = 4096
	maxLogLevelBytes     = 32
	maxRunIDLen          = 128
	maxErrorMsgBytes     = 8192
	// maxSlotsPerWorker bounds the slots count a worker can advertise on
	// registration. PickWorkerForQueue ranks by SlotsAvailable, so an
	// unbounded value lets a buggy or malicious worker monopolize dispatch
	// for its project. 1024 leaves several orders of magnitude of headroom
	// over realistic SDK concurrency (4–32 typical).
	maxSlotsPerWorker = 1024
	// Bounds for unconstrained string fields a worker advertises on
	// registration. Without these a misbehaving SDK can register with
	// megabyte-scale Hostname/SDK metadata, bloating the in-memory registry,
	// the dbSync UPSERT, and any audit row that captures the registration.
	// Limits are generous against typical real values (POSIX HOST_NAME_MAX is
	// 255; SDK versions and language tokens are short identifiers).
	maxHostnameBytes                  = 255
	maxSDKVersionBytes                = 64
	maxSDKLanguageBytes               = 32
	maxNameBytes                      = 128
	maxRegistrationMetadataEntries    = 64
	maxRegistrationMetadataKeyBytes   = 64
	maxRegistrationMetadataValueBytes = 512
)

var (
	errForceDisconnected             = errors.New("force disconnected by API request")
	errAPIKeyRevoked                 = errors.New("api key revoked")
	errAPIKeyExpired                 = errors.New("api key expired")
	errWorkerConnectionRenewalFailed = errors.New("worker connection reservation renewal failed")
)

func workerDisconnectChannel(projectID, workerID string) string {
	return "worker:disconnect:" + projectID + ":" + workerID
}

func workerDisconnectAckChannel(projectID, workerID string) string {
	return "worker:disconnect_ack:" + projectID + ":" + workerID
}

func apiKeyRevokedChannel(apiKeyID string) string {
	return "apikey:revoked:" + apiKeyID
}

func apiKeyExpiresChannel(apiKeyID string) string {
	return "apikey:expires:" + apiKeyID
}

func subscribeRequiredWorkerControlChannel(ctx context.Context, pub pubsub.Publisher, channel, purpose string) (*pubsub.Subscription, error) {
	if pub == nil {
		return nil, status.Errorf(codes.Unavailable, "worker stream %s subscription unavailable", purpose)
	}
	sub, err := pub.Subscribe(ctx, channel)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "worker stream %s subscription failed: %v", purpose, err)
	}
	if sub == nil {
		err = errors.New("nil subscription")
		return nil, status.Errorf(codes.Unavailable, "worker stream %s subscription failed: %v", purpose, err)
	}
	return sub, nil
}

func (s *workerService) ensureAPIKeyControlSubscriptions(
	ctx context.Context,
	apiKeyID string,
	revokeSub *pubsub.Subscription,
	expireSub *pubsub.Subscription,
) (*pubsub.Subscription, *pubsub.Subscription, error) {
	// The API key context is refreshed after the registration receive, so this
	// helper is safe to call both before and after the first worker message.
	if apiKeyID == "" {
		return revokeSub, expireSub, nil
	}
	if revokeSub == nil {
		revokeChannel := apiKeyRevokedChannel(apiKeyID)
		sub, err := subscribeRequiredWorkerControlChannel(ctx, s.pub, revokeChannel, "api key revocation")
		if err != nil {
			return nil, expireSub, err
		}
		revokeSub = sub
	}
	if expireSub == nil {
		expireChannel := apiKeyExpiresChannel(apiKeyID)
		sub, err := subscribeRequiredWorkerControlChannel(ctx, s.pub, expireChannel, "api key expiry")
		if err != nil {
			return revokeSub, nil, err
		}
		expireSub = sub
	}
	return revokeSub, expireSub, nil
}

// workerService implements workerv1.WorkerServiceServer.
type workerService struct {
	queries         *store.Queries
	pub             pubsub.Publisher
	registry        *ConnectionRegistry
	cfg             *config.Config
	resultChannels  *ResultChannelRegistry
	runFinalizer    *atomic.Value
	authLimiter     grpcAuthLimiter
	apiKeyResolver  apiKeyResolver
	billingEnforcer planLimitEnforcer
	edition         domain.Edition
	readyRunQueue   ReadyRunEnqueuer
}

// StreamTasks is the bidirectional streaming RPC between the server and a worker SDK.
//
// Protocol:
//  1. Client sends WorkerRegistration as first message.
//  2. Server registers the worker and begins dispatching TaskAssignment messages.
//  3. Client sends Heartbeat periodically to refresh last_seen_at.
//  4. Client sends TaskResult when a run completes or fails.
//  5. Client sends LogLine for in-flight run logs.
//  6. On disconnect: server deregisters the worker and emits an audit event.
func (s *workerService) StreamTasks(stream workerv1.WorkerService_StreamTasksServer) error {
	ctx := stream.Context()

	// Authenticate the connecting worker via the Bearer API key in gRPC metadata.
	apiKey, err := resolveAPIKeyFromContextWithLimitAndResolver(ctx, s.apiKeyLookupResolver(), s.authLimiter)
	if err != nil {
		return err
	}
	ctx = withAPIKeyContext(ctx, apiKey)
	projectID := apiKey.ProjectID
	pendingProjectID := projectID
	pendingAPIKeyID := apiKey.ID
	apiKeyExpiresAt, apiKeyHasExpiry := APIKeyExpiresAtFromContext(ctx)

	releasePending, err := s.reservePendingWorkerStream(pendingProjectID, pendingAPIKeyID)
	if err != nil {
		return err
	}
	defer func() {
		if releasePending != nil {
			releasePending()
		}
	}()

	var revokeKeySub *pubsub.Subscription
	var expireKeySub *pubsub.Subscription
	closeRevokeSubOnEarlyReturn := true
	closeExpireSubOnEarlyReturn := true
	defer func() {
		if closeRevokeSubOnEarlyReturn && revokeKeySub != nil {
			revokeKeySub.Close()
		}
		if closeExpireSubOnEarlyReturn && expireKeySub != nil {
			expireKeySub.Close()
		}
	}()
	revokeKeySub, expireKeySub, err = s.ensureAPIKeyControlSubscriptions(ctx, apiKey.ID, revokeKeySub, expireKeySub)
	if err != nil {
		return err
	}

	firstMsg, err := recvWorkerRegistrationMessage(ctx, stream, revokeKeySub, apiKey.ID, apiKeyExpiresAt, apiKeyHasExpiry)
	if err != nil {
		if errors.Is(err, errAPIKeyRevoked) || errors.Is(err, errAPIKeyExpired) {
			return err
		}
		return status.Errorf(codes.Internal, "recv registration: %v", err)
	}
	apiKey, err = resolveAPIKeyFromContextWithResolver(ctx, s.apiKeyLookupResolver())
	if err != nil {
		return err
	}
	ctx = withAPIKeyContext(ctx, apiKey)
	projectID = apiKey.ProjectID
	apiKeyExpiresAt, apiKeyHasExpiry = APIKeyExpiresAtFromContext(ctx)
	revokeKeySub, expireKeySub, err = s.ensureAPIKeyControlSubscriptions(ctx, apiKey.ID, revokeKeySub, expireKeySub)
	if err != nil {
		return err
	}
	reg, err := workerRegistrationFromFirstMessage(firstMsg)
	if err != nil {
		return err
	}
	if err := validateRegistration(reg); err != nil {
		return err
	}
	configureGRPCSentryWorkerScope(ctx, reg.WorkerId, reg.Name, reg.Hostname, reg.SdkLanguage, reg.SdkVersion)

	registered, releaseWorkerConnection, err := s.registerWorkerStream(ctx, apiKey, projectID, reg, pendingProjectID, pendingAPIKeyID)
	if err != nil {
		return err
	}
	defer releaseWorkerConnection()
	releasePending = nil
	recordWorkerStreamsOpen(ctx, reg.Queues, 1)

	// Reconcile in-flight tasks from the registration (reconnect recovery).
	// Passing workerID enables the adversarial ownership check.
	s.reconcileInFlightTasks(ctx, reg.WorkerId, projectID, reg.InFlightTasks)

	// Upsert worker into DB immediately (don't wait for the next sync tick).
	s.dbUpsertWorker(ctx, registered.worker)

	// Emit audit event.
	s.emitWorkerAudit(ctx, domain.AuditActionWorkerConnected, projectID, reg.WorkerId, map[string]any{
		"worker_id": reg.WorkerId,
		"hostname":  reg.Hostname,
		"queues":    reg.Queues,
	})

	slog.Info("grpc worker registered",
		"worker_id", reg.WorkerId,
		"project_id", projectID,
		"hostname", reg.Hostname,
		"queues", reg.Queues,
		"slots_total", reg.SlotsTotal,
	)

	sendWorkerRegistrationAck(stream, reg.WorkerId)

	// Clean up on any exit path. Pass the per-registration token so a stale
	// goroutine cannot evict a live replacement that registered under the
	// same WorkerID after a reconnect race.
	myToken := registered.worker.regToken
	var streamEndErr error
	defer func() {
		recordWorkerStreamsOpen(context.Background(), reg.Queues, -1)
		recordWorkerStreamDisconnect(context.Background(), streamDisconnectReason(streamEndErr))
		s.cleanupRegistration(projectID, reg.WorkerId, myToken)
	}()

	firstErr := s.runWorkerStreamLoops(workerStreamLoopConfig{
		ctx:                         ctx,
		stream:                      stream,
		registered:                  registered,
		registration:                reg,
		projectID:                   projectID,
		apiKeyID:                    apiKey.ID,
		apiKeyExpiresAt:             apiKeyExpiresAt,
		apiKeyHasExpiry:             apiKeyHasExpiry,
		revokeKeySub:                revokeKeySub,
		expireKeySub:                expireKeySub,
		closeRevokeSubOnEarlyReturn: &closeRevokeSubOnEarlyReturn,
		closeExpireSubOnEarlyReturn: &closeExpireSubOnEarlyReturn,
	})
	streamEndErr = firstErr
	s.cleanupRegistration(projectID, reg.WorkerId, myToken)
	return firstErr
}

type workerStreamLoopConfig struct {
	ctx                         context.Context
	stream                      workerv1.WorkerService_StreamTasksServer
	registered                  registeredWorkerStream
	registration                *workerv1.WorkerRegistration
	projectID                   string
	apiKeyID                    string
	apiKeyExpiresAt             time.Time
	apiKeyHasExpiry             bool
	revokeKeySub                *pubsub.Subscription
	expireKeySub                *pubsub.Subscription
	closeRevokeSubOnEarlyReturn *bool
	closeExpireSubOnEarlyReturn *bool
}

func (s *workerService) runWorkerStreamLoops(cfg workerStreamLoopConfig) error {
	reg := cfg.registration
	disconnectSub := s.subscribeWorkerDisconnect(cfg.ctx, cfg.projectID, reg.WorkerId)
	workerConnectionRenewer := s.workerConnectionRenewer(cfg.registered)

	var wg conc.WaitGroup
	streamErr := make(chan error, workerStreamGoroutineCount(
		disconnectSub,
		cfg.revokeKeySub,
		cfg.apiKeyHasExpiry || cfg.expireKeySub != nil,
		workerConnectionRenewer != nil,
	))

	if disconnectSub != nil {
		listenForWorkerForceDisconnect(cfg.ctx, &wg, streamErr, disconnectSub, reg.WorkerId, cfg.projectID)
	}
	if cfg.revokeKeySub != nil {
		*cfg.closeRevokeSubOnEarlyReturn = false
		s.listenForAPIKeyRevocation(cfg.ctx, &wg, streamErr, cfg.revokeKeySub, cfg.registered.worker, cfg.apiKeyID, reg.WorkerId, cfg.projectID)
	}
	if cfg.apiKeyHasExpiry || cfg.expireKeySub != nil {
		*cfg.closeExpireSubOnEarlyReturn = false
		s.listenForAPIKeyExpiry(cfg.ctx, &wg, streamErr, cfg.apiKeyExpiresAt, cfg.apiKeyHasExpiry, cfg.expireKeySub, cfg.registered.worker, cfg.apiKeyID, reg.WorkerId, cfg.projectID)
	}
	if workerConnectionRenewer != nil {
		s.startWorkerConnectionReservationRenewal(
			cfg.ctx,
			&wg,
			streamErr,
			workerConnectionRenewer,
			cfg.registered.orgID,
			cfg.registered.workerConnectionReservationID,
			reg.WorkerId,
		)
	}

	startWorkerSendLoop(cfg.ctx, &wg, streamErr, cfg.registered.sendCh, cfg.stream)
	s.startWorkerRecvLoop(
		cfg.ctx,
		&wg,
		streamErr,
		cfg.stream,
		reg.WorkerId,
		cfg.projectID,
		cfg.registered.orgID,
		cfg.registered.workerConnectionReservationID,
	)
	return <-streamErr
}

func (s *workerService) subscribeWorkerDisconnect(ctx context.Context, projectID, workerID string) *pubsub.Subscription {
	disconnectSub, err := s.pub.Subscribe(ctx, workerDisconnectChannel(projectID, workerID))
	if err != nil {
		slog.Warn("grpc: failed to subscribe to disconnect channel",
			"worker_id", workerID,
			"error", err,
		)
		return nil
	}
	return disconnectSub
}

func (s *workerService) workerConnectionRenewer(registered registeredWorkerStream) workerConnectionReservationEnforcer {
	reserver, ok := s.billingEnforcer.(workerConnectionReservationEnforcer)
	if !ok || registered.orgID == "" || registered.workerConnectionReservationID == "" {
		return nil
	}
	return reserver
}

type registeredWorkerStream struct {
	worker                        *ConnectedWorker
	sendCh                        chan *workerv1.ServerMessage
	orgID                         string
	workerConnectionReservationID string
}

func (s *workerService) registerWorkerStream(
	ctx context.Context,
	apiKey *domain.APIKey,
	projectID string,
	reg *workerv1.WorkerRegistration,
	pendingProjectID string,
	pendingAPIKeyID string,
) (registeredWorkerStream, func(), error) {
	workerConnectionReservationID := uuid.Must(uuid.NewV7()).String()
	orgID, releaseWorkerConnection, err := s.checkPlanConnectionLimit(ctx, projectID, workerConnectionReservationID)
	if err != nil {
		return registeredWorkerStream{}, nil, err
	}

	sendCh := make(chan *workerv1.ServerMessage, 32)
	cw := newConnectedWorkerFromRegistration(reg, apiKey, projectID, orgID, sendCh)
	s.registry.ReleasePendingStream(pendingProjectID, pendingAPIKeyID)
	if err := s.registry.Register(cw); err != nil {
		releaseWorkerConnection()
		if errors.Is(err, ErrWorkerStreamQuotaExceeded) {
			return registeredWorkerStream{}, nil, status.Errorf(codes.ResourceExhausted, "register worker: %v", err)
		}
		return registeredWorkerStream{}, nil, status.Errorf(codes.AlreadyExists, "register worker: %v", err)
	}

	return registeredWorkerStream{
		worker:                        cw,
		sendCh:                        sendCh,
		orgID:                         orgID,
		workerConnectionReservationID: workerConnectionReservationID,
	}, releaseWorkerConnection, nil
}

func workerRegistrationFromFirstMessage(msg *workerv1.WorkerMessage) (*workerv1.WorkerRegistration, error) {
	if msg == nil {
		return nil, status.Error(codes.InvalidArgument, "first message must be WorkerRegistration")
	}
	regPayload, ok := msg.Payload.(*workerv1.WorkerMessage_Registration)
	if !ok || regPayload.Registration == nil {
		return nil, status.Error(codes.InvalidArgument, "first message must be WorkerRegistration")
	}
	return regPayload.Registration, nil
}

func (s *workerService) apiKeyLookupResolver() apiKeyResolver {
	if s == nil {
		return nil
	}
	if s.apiKeyResolver != nil {
		return s.apiKeyResolver
	}
	return queryAPIKeyResolver(s.queries)
}

func workerStreamGoroutineCount(disconnectSub, revokeKeySub *pubsub.Subscription, hasExpiry, renewsWorkerConnection bool) int {
	goroutineCount := 2
	if disconnectSub != nil {
		goroutineCount++
	}
	if revokeKeySub != nil {
		goroutineCount++
	}
	if hasExpiry {
		goroutineCount++
	}
	if renewsWorkerConnection {
		goroutineCount++
	}
	return goroutineCount
}

func sendWorkerRegistrationAck(stream workerv1.WorkerService_StreamTasksServer, workerID string) {
	_ = stream.Send(&workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_Ack{
			Ack: &workerv1.Acknowledged{Id: workerID},
		},
	})
}

func newConnectedWorkerFromRegistration(
	reg *workerv1.WorkerRegistration,
	apiKey *domain.APIKey,
	projectID string,
	orgID string,
	sendCh chan *workerv1.ServerMessage,
) *ConnectedWorker {
	return &ConnectedWorker{
		WorkerID:       reg.WorkerId,
		ProjectID:      projectID,
		OrgID:          orgID,
		EnvironmentID:  apiKey.EnvironmentID,
		APIKeyID:       apiKey.ID,
		Name:           reg.Name,
		Hostname:       reg.Hostname,
		SDKVersion:     reg.SdkVersion,
		SDKLanguage:    reg.SdkLanguage,
		Queues:         reg.Queues,
		SlotsTotal:     reg.SlotsTotal,
		SlotsAvailable: reg.SlotsAvailable,
		Status:         "active",
		SendCh:         sendCh,
		revokeCh:       make(chan struct{}),
	}
}

func startWorkerSendLoop(
	ctx context.Context,
	wg *conc.WaitGroup,
	streamErr chan<- error,
	sendCh <-chan *workerv1.ServerMessage,
	stream workerv1.WorkerService_StreamTasksServer,
) {
	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				streamErr <- nil
				return
			case msg, open := <-sendCh:
				if !open {
					streamErr <- nil
					return
				}
				if err := stream.Send(msg); err != nil {
					streamErr <- fmt.Errorf("send: %w", err)
					return
				}
			}
		}
	})
}

func (s *workerService) startWorkerRecvLoop(
	ctx context.Context,
	wg *conc.WaitGroup,
	streamErr chan<- error,
	stream workerv1.WorkerService_StreamTasksServer,
	workerID string,
	projectID string,
	orgID string,
	workerConnectionReservationID string,
) {
	wg.Go(func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				streamErr <- err
				return
			}
			if err := s.handleWorkerMessage(ctx, workerID, projectID, orgID, workerConnectionReservationID, msg); err != nil {
				slog.Warn("grpc handle worker message error",
					"worker_id", workerID,
					"error", err,
				)
			}
		}
	})
}

func listenForWorkerForceDisconnect(
	ctx context.Context,
	wg *conc.WaitGroup,
	streamErr chan<- error,
	disconnectSub *pubsub.Subscription,
	workerID string,
	projectID string,
) {
	wg.Go(func() {
		defer disconnectSub.Close()
		select {
		case <-ctx.Done():
			streamErr <- nil
		case <-disconnectSub.Ch:
			slog.Info("grpc worker force-disconnect received",
				"worker_id", workerID,
				"project_id", projectID,
			)
			streamErr <- errForceDisconnected
		}
	})
}

func (s *workerService) startWorkerConnectionReservationRenewal(
	ctx context.Context,
	wg *conc.WaitGroup,
	streamErr chan<- error,
	reserver workerConnectionReservationEnforcer,
	orgID string,
	reservationID string,
	workerID string,
) {
	lease := s.workerConnectionLease()
	interval := max(lease/3, 10*time.Millisecond)
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				streamErr <- nil
				return
			case <-ticker.C:
				if err := reserver.RenewWorkerConnection(ctx, orgID, reservationID, lease); err != nil {
					slog.Warn("grpc worker connection reservation renewal failed; closing stream",
						"worker_id", workerID,
						"org_id", orgID,
						"error", err,
					)
					streamErr <- errWorkerConnectionRenewalFailed
					return
				}
			}
		}
	})
}

func (s *workerService) listenForAPIKeyRevocation(
	ctx context.Context,
	wg *conc.WaitGroup,
	streamErr chan<- error,
	revokeKeySub *pubsub.Subscription,
	cw *ConnectedWorker,
	apiKeyID string,
	workerID string,
	projectID string,
) {
	wg.Go(func() {
		defer revokeKeySub.Close()
		select {
		case <-ctx.Done():
			streamErr <- nil
		case <-revokeKeySub.Ch:
			slog.Info("grpc worker api key revoked, closing stream",
				"worker_id", workerID,
				"api_key_id", apiKeyID,
				"project_id", projectID,
			)
			// Also close via registry so co-located streams for the same key are notified.
			s.registry.CloseByAPIKey(apiKeyID)
			streamErr <- errAPIKeyRevoked
		case <-cw.revokeCh:
			// Triggered locally by registry.CloseByAPIKey from another goroutine.
			slog.Info("grpc worker api key revoked (local signal), closing stream",
				"worker_id", workerID,
				"api_key_id", apiKeyID,
			)
			streamErr <- errAPIKeyRevoked
		}
	})
}

func (s *workerService) listenForAPIKeyExpiry(
	ctx context.Context,
	wg *conc.WaitGroup,
	streamErr chan<- error,
	expiresAt time.Time,
	hasExpiry bool,
	expireKeySub *pubsub.Subscription,
	_ *ConnectedWorker,
	apiKeyID string,
	workerID string,
	projectID string,
) {
	wg.Go(func() {
		var timer *time.Timer
		var timerCh <-chan time.Time
		resetTimer := func(deadline time.Time) bool {
			wait := time.Until(deadline)
			if wait <= 0 {
				return false
			}
			if timer == nil {
				timer = time.NewTimer(wait)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(wait)
			}
			timerCh = timer.C
			return true
		}
		if hasExpiry && !resetTimer(expiresAt) {
			streamErr <- errAPIKeyExpired
			s.registry.CloseByAPIKey(apiKeyID)
			return
		}
		defer func() {
			if timer != nil {
				timer.Stop()
			}
			if expireKeySub != nil {
				expireKeySub.Close()
			}
		}()
		var expireSignalCh <-chan []byte
		if expireKeySub != nil {
			expireSignalCh = expireKeySub.Ch
		}
		for {
			select {
			case <-ctx.Done():
				streamErr <- nil
				return
			case payload, ok := <-expireSignalCh:
				if !ok {
					expireSignalCh = nil
					continue
				}
				nextExpiry, err := parseAPIKeyExpirySignal(payload)
				if err != nil {
					slog.Warn("grpc worker api key expiry signal invalid, closing stream",
						"worker_id", workerID,
						"api_key_id", apiKeyID,
						"project_id", projectID,
						"error", err,
					)
					s.registry.CloseByAPIKey(apiKeyID)
					streamErr <- errAPIKeyExpired
					return
				}
				if !resetTimer(nextExpiry) {
					streamErr <- errAPIKeyExpired
					s.registry.CloseByAPIKey(apiKeyID)
					return
				}
			case <-timerCh:
				slog.Info("grpc worker api key rotation grace expired, closing stream",
					"worker_id", workerID,
					"api_key_id", apiKeyID,
					"project_id", projectID,
				)
				streamErr <- errAPIKeyExpired
				s.registry.CloseByAPIKey(apiKeyID)
				return
			}
		}
	})
}

func parseAPIKeyExpirySignal(payload []byte) (time.Time, error) {
	deadline, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(payload)))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse api key expiry signal: %w", err)
	}
	return deadline, nil
}

func streamDisconnectReason(err error) string {
	switch {
	case err == nil:
		return "graceful"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "graceful"
	case errors.Is(err, errForceDisconnected):
		return "forced"
	case errors.Is(err, errAPIKeyRevoked):
		return "revoked"
	case errors.Is(err, errAPIKeyExpired):
		return "expired"
	case errors.Is(err, errWorkerConnectionRenewalFailed):
		return "worker_connection_reservation_lost"
	default:
		return "error"
	}
}

// checkPlanConnectionLimit resolves the org for the supplied project and
// rejects the registration with codes.ResourceExhausted if the org is at
// or above its plan-tier worker connection cap. Returns the resolved orgID
// (which may be empty only when the edition is ungated) so the caller can
// attach it to the ConnectedWorker entry. Existing connections are never
// evicted; this is a connect-time gate only.
func (s *workerService) checkPlanConnectionLimit(ctx context.Context, projectID, reservationID string) (string, func(), error) {
	releaseNoop := func() {}
	if s.edition == domain.EditionCommunity {
		return "", releaseNoop, nil
	}
	if s.billingEnforcer == nil {
		slog.Error("grpc registration: billing enforcer missing in gated edition", "project_id", projectID)
		return "", releaseNoop, status.Error(codes.Unavailable, "worker connection plan lookup unavailable")
	}
	orgID, orgErr := s.billingEnforcer.GetActiveProjectOrgID(ctx, projectID)
	if orgErr != nil {
		slog.Error("grpc registration: project org lookup failed, failing closed",
			"project_id", projectID, "error", orgErr)
		return "", releaseNoop, status.Error(codes.Unavailable, "worker connection plan lookup unavailable")
	}
	if orgID == "" {
		slog.Error("grpc registration: project org lookup returned empty org, failing closed",
			"project_id", projectID)
		return "", releaseNoop, status.Error(codes.Unavailable, "worker connection plan lookup unavailable")
	}
	if reserver, ok := s.billingEnforcer.(workerConnectionReservationEnforcer); ok {
		release, err := reserver.ReserveWorkerConnection(ctx, orgID, reservationID, s.workerConnectionLease())
		if err != nil {
			return orgID, releaseNoop, status.Errorf(codes.ResourceExhausted, "%v", err)
		}
		return orgID, release, nil
	}
	currentActive := s.registry.CountByOrg(orgID)
	if err := s.billingEnforcer.CheckWorkerConnectionLimit(ctx, orgID, currentActive); err != nil {
		return orgID, releaseNoop, status.Errorf(codes.ResourceExhausted, "%v", err)
	}
	return orgID, releaseNoop, nil
}

func (s *workerService) workerConnectionLease() time.Duration {
	if s.cfg != nil && s.cfg.WorkerHeartbeatTimeout > 0 {
		return s.cfg.WorkerHeartbeatTimeout * 3
	}
	return 90 * time.Second
}

func (s *workerService) reservePendingWorkerStream(projectID, apiKeyID string) (func(), error) {
	if err := s.registry.ReservePendingStream(projectID, apiKeyID); err != nil {
		if errors.Is(err, ErrWorkerStreamQuotaExceeded) {
			return nil, status.Errorf(codes.ResourceExhausted, "reserve worker stream: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "reserve worker stream: %v", err)
	}

	return func() {
		s.registry.ReleasePendingStream(projectID, apiKeyID)
	}, nil
}

func recvWorkerRegistrationMessage(
	ctx context.Context,
	stream workerv1.WorkerService_StreamTasksServer,
	revokeKeySub *pubsub.Subscription,
	apiKeyID string,
	apiKeyExpiresAt time.Time,
	apiKeyHasExpiry bool,
) (*workerv1.WorkerMessage, error) {
	if revokeKeySub == nil && !apiKeyHasExpiry {
		return stream.Recv()
	}
	if apiKeyHasExpiry && !apiKeyExpiresAt.After(time.Now()) {
		return nil, errAPIKeyExpired
	}

	var expiryCh <-chan time.Time
	var expiryTimer *time.Timer
	if apiKeyHasExpiry {
		expiryTimer = time.NewTimer(time.Until(apiKeyExpiresAt))
		expiryCh = expiryTimer.C
		defer expiryTimer.Stop()
	}
	var revokeCh <-chan []byte
	if revokeKeySub != nil {
		revokeCh = revokeKeySub.Ch
	}

	type recvResult struct {
		msg *workerv1.WorkerMessage
		err error
	}
	recvCh := make(chan recvResult, 1)
	var recvWG conc.WaitGroup
	recvWG.Go(func() {
		msg, err := stream.Recv()
		recvCh <- recvResult{msg: msg, err: err}
	})

	select {
	case res := <-recvCh:
		return res.msg, res.err
	case <-revokeCh:
		slog.Info("grpc worker api key revoked before registration", "api_key_id", apiKeyID)
		return nil, errAPIKeyRevoked
	case <-expiryCh:
		slog.Info("grpc worker api key expired before registration", "api_key_id", apiKeyID)
		return nil, errAPIKeyExpired
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// cleanupRegistration performs stream-exit cleanup only for the connection
// that still owns the live registry entry. Stale streams from same-ID
// reconnect races must not mark a replacement worker offline or requeue its
// assignments.
func (s *workerService) cleanupRegistration(projectID, workerID string, token uint64) {
	if s.registry.Deregister(workerID, token) {
		s.finalizeDisconnect(projectID, workerID)
	}
}

// handleWorkerMessage dispatches an incoming WorkerMessage to the appropriate handler.
func (s *workerService) handleWorkerMessage(ctx context.Context, workerID, projectID, orgID, workerConnectionReservationID string, msg *workerv1.WorkerMessage) error {
	switch p := msg.Payload.(type) {
	case *workerv1.WorkerMessage_Heartbeat:
		return s.handleHeartbeat(ctx, workerID, projectID, orgID, workerConnectionReservationID, p.Heartbeat)
	case *workerv1.WorkerMessage_TaskResult:
		return s.handleTaskResult(ctx, workerID, projectID, p.TaskResult)
	case *workerv1.WorkerMessage_LogLine:
		return s.handleLogLine(ctx, workerID, projectID, p.LogLine)
	case *workerv1.WorkerMessage_Ack:
		return s.handleAck(ctx, workerID, projectID, p.Ack)
	case *workerv1.WorkerMessage_Registration:
		// Re-registration on an established stream is ignored (handled at connect).
		return nil
	default:
		return nil
	}
}

func (s *workerService) handleAck(ctx context.Context, workerID, projectID string, ack *workerv1.Acknowledged) error {
	if ack == nil || ack.Id == "" {
		return nil
	}
	if len(ack.Id) > maxRunIDLen {
		slog.Warn("grpc ack: run_id exceeds bound — rejecting",
			"worker_id", workerID, "run_id_len", len(ack.Id))
		return nil
	}
	task, err := s.queries.GetOpenWorkerTaskByRunID(ctx, workerID, projectID, ack.Id)
	if err != nil {
		return err
	}
	if task == nil || !canAcknowledgeWorkerTaskStatus(task.Status) {
		return nil
	}
	return s.queries.UpdateWorkerTaskStatus(ctx, task.ID, domain.WorkerTaskStatusAccepted)
}

func canAcknowledgeWorkerTaskStatus(status domain.WorkerTaskStatus) bool {
	return status == domain.WorkerTaskStatusAssigned
}

// handleHeartbeat is a no-op on the DB. last_seen_at is refreshed by the
// dbSync loop (RegisterWorker UPSERT, every WORKER_DB_SYNC_INTERVAL ≈ 15s),
// which is well inside the WORKER_HEARTBEAT_TIMEOUT sweep window (≈ 30s).
// Writing on every heartbeat caused N×workers DB writes per HeartbeatInterval
// without changing observability — the dbSync row already carries the same
// timestamp. The slot hint in hb is informational; the server is
// authoritative on slot accounting via Increment/DecrementSlots.
func (s *workerService) handleHeartbeat(ctx context.Context, workerID, projectID, orgID, workerConnectionReservationID string, hb *workerv1.Heartbeat) error {
	if hb == nil {
		return nil
	}
	if err := s.queries.RenewWorkerStreamLease(ctx, workerID, projectID, time.Now().Add(s.workerConnectionLease())); err != nil {
		slog.Warn("grpc heartbeat: failed to renew worker stream lease",
			"worker_id", workerID, "project_id", projectID, "error", err)
	}
	if reserver, ok := s.billingEnforcer.(workerConnectionReservationEnforcer); ok && orgID != "" && workerConnectionReservationID != "" {
		if err := reserver.RenewWorkerConnection(ctx, orgID, workerConnectionReservationID, s.workerConnectionLease()); err != nil {
			slog.Warn("grpc heartbeat: failed to renew worker connection reservation",
				"worker_id", workerID, "org_id", orgID, "error", err)
		}
	}
	return nil
}

// handleTaskResult reconciles a completed/failed run from the worker.
// If a WorkerDispatch call is waiting on this run, the result is routed via
// the ResultChannelRegistry so the dispatch goroutine can handle terminal
// state transitions (status update, cost recording). If no channel is
// registered (e.g. the dispatcher timed out), this method falls back to
// updating the run status directly.
func (s *workerService) handleTaskResult(ctx context.Context, workerID, projectID string, tr *workerv1.TaskResult) error {
	if tr == nil || tr.RunId == "" {
		return nil
	}
	// Bound RunId so a malicious worker can't use it as a pubsub-channel
	// amplifier or DB-key blow-up vector.
	if len(tr.RunId) > maxRunIDLen {
		slog.Warn("grpc task result: run_id exceeds bound — rejecting",
			"worker_id", workerID, "run_id_len", len(tr.RunId))
		return nil
	}
	if tr.AssignmentId == "" || tr.Attempt <= 0 {
		slog.Warn("grpc task result: missing assignment identity - rejecting",
			"worker_id", workerID,
			"run_id", tr.RunId,
		)
		return nil
	}
	// Cap error message so a worker can't bloat DB rows or page logs.
	if len(tr.ErrorMessage) > maxErrorMsgBytes {
		tr.ErrorMessage = tr.ErrorMessage[:maxErrorMsgBytes]
	}
	if taskResultOutputInvalid(tr.Status, tr.OutputJson) {
		slog.Warn("grpc task result: invalid output_json for success result - treating as failure",
			"worker_id", workerID,
			"run_id", tr.RunId,
		)
		tr.Status = "failed"
		tr.ErrorMessage = invalidWorkerOutputError
		tr.OutputJson = nil
	}

	// Route result to a waiting WorkerDispatch call if one exists.
	// The dispatch goroutine is responsible for slot accounting in that path.
	// The result channel is project-scoped so a worker authenticated to a
	// different project cannot deliver a forged TaskResult into another
	// project's dispatch goroutine: Send drops the message on project mismatch.
	if s.resultChannels != nil {
		sent, sendErr := s.resultChannels.SendAfterHandoff(tr.RunId, projectID, workerID, tr, func() (bool, error) {
			return s.queries.MarkWorkerTaskResultReceivedByAssignment(
				ctx,
				tr.AssignmentId,
				workerID,
				projectID,
				tr.RunId,
				int(tr.Attempt),
				tr.Status,
				tr.ErrorMessage,
				copyJSONBytes(tr.OutputJson),
				tr.DurationMs,
			)
		})
		if sendErr != nil {
			slog.Warn("grpc task result: result handoff failed",
				"worker_id", workerID,
				"run_id", tr.RunId,
				"error", sendErr,
			)
			return nil
		}
		if sent {
			// Successfully delivered to the waiting dispatcher — it owns the rest.
			return nil
		}
	}

	// No dispatcher is waiting (e.g. timed out or disconnected mid-flight)
	// OR the message was dropped above due to a project mismatch.
	// Adversarial guard: confirm the run belongs to this worker's project before
	// touching status. Without this check, a worker authenticated to project A
	// could mark runs in project B if it knew (or guessed) the run ID.
	run, err := s.queries.GetRun(ctx, tr.RunId)
	if err != nil || run == nil {
		slog.Warn("grpc task result: get run failed",
			"worker_id", workerID, "run_id", tr.RunId, "error", err)
		return nil
	}
	if run.ProjectID != projectID {
		slog.Warn("grpc task result: project mismatch — rejecting",
			"worker_id", workerID, "run_id", tr.RunId,
			"worker_project", projectID, "run_project", run.ProjectID)
		return nil
	}

	// Ownership guard: confirm the worker_tasks row exists and belongs to this
	// worker. This mirrors handleLogLine so a worker cannot mark runs it was
	// never assigned. The row also gives us the task ID needed to drive the
	// worker_tasks transition below.
	taskRow, taskErr := s.queries.GetOpenWorkerTaskByAssignment(ctx, tr.AssignmentId, workerID, projectID, tr.RunId, int(tr.Attempt))
	if taskErr != nil {
		slog.Warn("grpc task result fallback: ownership lookup failed",
			"worker_id", workerID, "run_id", tr.RunId, "error", taskErr)
		return nil
	}
	if taskRow == nil {
		slog.Warn("grpc task result fallback: rejecting — run not assigned to this worker",
			"worker_id", workerID, "run_id", tr.RunId)
		return nil
	}

	if finalizer := s.finalizer(); finalizer != nil {
		taskStatus, finalizerErr := finalizer.FinalizeWorkerRunResult(ctx, tr.RunId, tr.Status, tr.ErrorMessage, copyJSONBytes(tr.OutputJson))
		if finalizerErr != nil {
			slog.Warn("grpc task result fallback: finalizer failed",
				"worker_id", workerID,
				"run_id", tr.RunId,
				"error", finalizerErr,
			)
			return nil
		}
		if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, taskStatus); err != nil {
			slog.Warn("grpc task result fallback: update worker_task status failed",
				"task_id", taskRow.ID,
				"run_id", tr.RunId,
				"status", taskStatus,
				"error", err,
			)
		}
		return nil
	}

	// Fall back: update the run status directly. Do NOT restore the slot
	// here — if a WorkerDispatch goroutine ever held this run, it has
	// already restored the slot on its ctx.Done() / result branch
	// (see dispatch.go IncrementSlots sites). If no dispatcher ever held
	// the run on this replica (e.g. cross-replica handoff, in-flight
	// reconnect), no slot was decremented here, so there is nothing to
	// credit. Calling IncrementSlots in this path produced an over-credit
	// when a late result arrived after the dispatcher's ctx.Done()
	// already restored the slot, letting the worker monopolize dispatch.

	var newStatus domain.RunStatus
	var newTaskStatus domain.WorkerTaskStatus
	switch tr.Status {
	case "success":
		newStatus = domain.StatusCompleted
		newTaskStatus = domain.WorkerTaskStatusCompleted
	case "failed":
		newStatus = domain.StatusFailed
		newTaskStatus = domain.WorkerTaskStatusFailed
	default:
		newStatus = domain.StatusFailed
		newTaskStatus = domain.WorkerTaskStatusFailed
	}

	// Transition the run to its terminal state.
	finishedAt := time.Now()
	fields := map[string]any{"finished_at": finishedAt}
	if tr.ErrorMessage != "" {
		fields["error"] = tr.ErrorMessage
	}
	if tr.Status == "success" && len(tr.OutputJson) > 0 {
		out := make([]byte, len(tr.OutputJson))
		copy(out, tr.OutputJson)
		fields["result"] = json.RawMessage(out)
	}
	if err := s.queries.UpdateRunStatus(ctx, tr.RunId, domain.StatusExecuting, newStatus, fields); err != nil {
		slog.Warn("grpc task result: update run status failed",
			"run_id", tr.RunId,
			"status", newStatus,
			"error", err,
		)
		return nil
	}

	// Transition the worker_tasks row to its terminal state. The normal dispatch
	// path (dispatch.go) does this when the result arrives in time; the fallback
	// must do it too so a late TaskResult doesn't leave the row stuck in
	// "assigned" forever. UpdateWorkerTaskStatus is idempotent — safe to call
	// even if a concurrent normal-path update already wrote the same value.
	if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, newTaskStatus); err != nil {
		slog.Warn("grpc task result fallback: update worker_task status failed",
			"task_id", taskRow.ID,
			"run_id", tr.RunId,
			"status", newTaskStatus,
			"error", err,
		)
	}

	// Publish result to the per-run pub/sub channel so SSE subscribers get notified.
	type runResultEvent struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	payload, _ := json.Marshal(runResultEvent{RunID: tr.RunId, Status: string(newStatus)})
	if err := s.pub.Publish(ctx, grpcRunPubSubChannel(tr.RunId), payload); err != nil {
		slog.Warn("grpc task result: publish failed", "run_id", tr.RunId, "error", err)
	}

	return nil
}

// handleLogLine publishes a worker log line to the per-run pub/sub channel.
//
// Adversarial guard: the worker may only emit logs for runs assigned to it
// via worker_tasks (the same row written by WorkerDispatch). Without this
// check, a worker authenticated to project A could publish forged log lines
// to any run in any project — visible via the SSE log stream.
// allowedWorkerLogLevels is the set of log levels a worker may report. Anything
// else is normalized to "info".
var allowedWorkerLogLevels = map[string]struct{}{
	"trace": {}, "debug": {}, "info": {}, "warn": {}, "warning": {}, "error": {}, "fatal": {},
}

func normalizeWorkerLogLevel(level string) string {
	l := strings.ToLower(strings.TrimSpace(level))
	if _, ok := allowedWorkerLogLevels[l]; ok {
		return l
	}
	return "info"
}

// sanitizeWorkerLogTimestamp returns the worker-provided millisecond timestamp
// when it is within a plausible window of now, otherwise the server's current
// time. Bounds reject non-positive, far-future, and stale/replayed timestamps.
func sanitizeWorkerLogTimestamp(tsMillis int64, now time.Time) int64 {
	const maxFutureSkewMs = int64(24 * 60 * 60 * 1000)   // 1 day
	const maxPastAgeMs = int64(30 * 24 * 60 * 60 * 1000) // 30 days
	nowMs := now.UnixMilli()
	if tsMillis <= 0 || tsMillis > nowMs+maxFutureSkewMs || tsMillis < nowMs-maxPastAgeMs {
		return nowMs
	}
	return tsMillis
}

func (s *workerService) handleLogLine(ctx context.Context, workerID, projectID string, ll *workerv1.LogLine) error {
	if ll == nil || ll.RunId == "" {
		return nil
	}
	if len(ll.RunId) > maxRunIDLen {
		slog.Warn("grpc log line: run_id exceeds bound — rejecting",
			"worker_id", workerID, "run_id_len", len(ll.RunId))
		return nil
	}
	taskRow, err := s.queries.GetOpenWorkerTaskByRunID(ctx, workerID, projectID, ll.RunId)
	if err != nil {
		slog.Warn("grpc log line: ownership lookup failed",
			"worker_id", workerID, "run_id", ll.RunId, "error", err)
		return nil
	}
	if taskRow == nil {
		slog.Warn("grpc log line: rejecting — run not assigned to this worker",
			"worker_id", workerID, "run_id", ll.RunId)
		return nil
	}
	msg := ll.Message
	if len(msg) > maxLogMessageBytes {
		msg = msg[:maxLogMessageBytes]
	}
	level := ll.Level
	if len(level) > maxLogLevelBytes {
		level = level[:maxLogLevelBytes]
	}
	// Worker-supplied level is free-form over the wire; normalize it to a known
	// allowlist so a hostile worker cannot inject arbitrary level strings that
	// downstream SSE consumers might mis-render or use to spoof severity.
	level = normalizeWorkerLogLevel(level)
	type logLineEvent struct {
		RunID     string `json:"run_id"`
		Level     string `json:"level"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp_unix_ms"`
	}
	payload, _ := json.Marshal(logLineEvent{
		RunID:   ll.RunId,
		Level:   level,
		Message: msg,
		// timestamp_unix_ms is fully worker-controlled; clamp implausible values
		// (non-positive, far-future, or stale/replayed) to server time so they
		// cannot corrupt downstream ordering.
		Timestamp: sanitizeWorkerLogTimestamp(ll.TimestampUnixMs, time.Now()),
	})
	channel := grpcWorkerLogChannel(ll.RunId)
	if err := s.pub.Publish(ctx, channel, payload); err != nil {
		slog.Warn("grpc log line publish failed", "run_id", ll.RunId, "error", err)
	}
	return nil
}

func grpcRunPubSubChannel(runID string) string {
	return "run:" + runID
}

func grpcWorkerLogChannel(runID string) string {
	return "worker:log:" + runID
}

// reconcileInFlightTasks handles runs that the worker was executing at the time
// of (re)connection. It applies the correct terminal transition per status and,
// for failed/abandoned runs, schedules a retry per the job's retry policy
// (mirroring the executor's handleFailure path).
//
// Adversarial guard: before reconciling, the run is validated against
// worker_tasks to confirm it was actually assigned to this worker. Mismatches
// are logged and skipped — this prevents a malicious or buggy worker from
// marking runs it doesn't own.
func (s *workerService) reconcileInFlightTasks(ctx context.Context, workerID, projectID string, tasks []*workerv1.InFlightTask) {
	for _, t := range tasks {
		if t == nil || t.RunId == "" {
			continue
		}
		if len(t.RunId) > maxRunIDLen {
			slog.Warn("grpc reconcile: run_id exceeds bound — skipping",
				"worker_id", workerID, "run_id_len", len(t.RunId))
			continue
		}
		if t.AssignmentId == "" || t.Attempt <= 0 {
			slog.Warn("grpc reconcile: missing assignment identity - skipping",
				"worker_id", workerID,
				"run_id", t.RunId,
			)
			continue
		}
		if len(t.ErrorMessage) > maxErrorMsgBytes {
			t.ErrorMessage = t.ErrorMessage[:maxErrorMsgBytes]
		}
		if taskResultOutputInvalid(t.Status, t.OutputJson) {
			slog.Warn("grpc reconcile: invalid output_json for completed task - treating as failure",
				"worker_id", workerID,
				"run_id", t.RunId,
			)
			t.Status = "failed"
			t.ErrorMessage = invalidWorkerOutputError
			t.OutputJson = nil
		}

		// Adversarial guard: verify ownership via worker_tasks.
		taskRow, err := s.queries.GetOpenWorkerTaskByAssignment(ctx, t.AssignmentId, workerID, projectID, t.RunId, int(t.Attempt))
		if err != nil {
			slog.Warn("grpc reconcile: ownership lookup failed",
				"worker_id", workerID,
				"run_id", t.RunId,
				"error", err,
			)
			continue
		}
		if taskRow == nil {
			// No matching worker_tasks row — mismatch; reject.
			slog.Warn("grpc reconcile: rejecting in-flight task not owned by this assignment",
				"worker_id", workerID,
				"run_id", t.RunId,
				"assignment_id", t.AssignmentId,
				"attempt", t.Attempt,
			)
			continue
		}

		switch t.Status {
		case "completed":
			taskStatus := domain.WorkerTaskStatusCompleted
			if finalizer := s.finalizer(); finalizer != nil {
				var finalizerErr error
				taskStatus, finalizerErr = finalizer.FinalizeWorkerRunResult(ctx, t.RunId, "success", "", copyJSONBytes(t.OutputJson))
				if finalizerErr != nil {
					slog.Warn("grpc reconcile: finalizer failed",
						"run_id", t.RunId,
						"error", finalizerErr,
					)
					continue
				}
			} else {
				reconcileFields := map[string]any{"finished_at": time.Now()}
				if len(t.OutputJson) > 0 {
					reconcileFields["result"] = copyJSONBytes(t.OutputJson)
				}
				if err := s.queries.UpdateRunStatus(ctx, t.RunId, domain.StatusExecuting, domain.StatusCompleted, reconcileFields); err != nil {
					slog.Warn("grpc reconcile: mark completed failed",
						"run_id", t.RunId,
						"error", err,
					)
					continue
				}
			}
			if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, taskStatus); err != nil {
				slog.Warn("grpc reconcile: update worker_task status failed",
					"task_id", taskRow.ID,
					"run_id", t.RunId,
					"error", err,
				)
			}

		case "failed", "abandoned":
			taskStatus := domain.WorkerTaskStatusFailed
			if finalizer := s.finalizer(); finalizer != nil {
				var finalizerErr error
				taskStatus, finalizerErr = finalizer.FinalizeWorkerRunResult(ctx, t.RunId, "failed", t.ErrorMessage, nil)
				if finalizerErr != nil {
					slog.Warn("grpc reconcile: finalizer failed",
						"run_id", t.RunId,
						"error", finalizerErr,
					)
					continue
				}
			} else {
				// For failed/abandoned: attempt a retry if the job allows it,
				// otherwise mark as dead-letter.
				s.reconcileFailedTask(ctx, t)
			}
			// Whether the run gets requeued or dead-lettered, the worker_task
			// row that recorded this assignment is done — mark it failed so it
			// doesn't linger in "assigned" forever.
			if err := s.queries.UpdateWorkerTaskStatus(ctx, taskRow.ID, taskStatus); err != nil {
				slog.Warn("grpc reconcile: update worker_task status failed",
					"task_id", taskRow.ID,
					"run_id", t.RunId,
					"error", err,
				)
			}

		default:
			slog.Warn("grpc reconcile: unknown in-flight task status",
				"worker_id", workerID,
				"run_id", t.RunId,
				"status", t.Status,
			)
		}
	}
}

func (s *workerService) finalizer() WorkerRunResultFinalizer {
	if s.runFinalizer == nil {
		return nil
	}
	v := s.runFinalizer.Load()
	if v == nil {
		return nil
	}
	finalizer, _ := v.(WorkerRunResultFinalizer)
	return finalizer
}

func copyJSONBytes(in []byte) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return json.RawMessage(out)
}

// reconcileFailedTask applies retry-or-fail logic for a failed/abandoned run
// reported during worker reconnection. It mirrors the retry scheduling in
// internal/worker/executor_handlers.go without importing that package.
func (s *workerService) reconcileFailedTask(ctx context.Context, t *workerv1.InFlightTask) {
	run, err := s.queries.GetRun(ctx, t.RunId)
	if err != nil || run == nil {
		slog.Warn("grpc reconcile: get run failed",
			"run_id", t.RunId,
			"error", err,
		)
		return
	}

	job, err := s.queries.GetJob(ctx, run.JobID)
	if err != nil || job == nil {
		slog.Warn("grpc reconcile: get job failed",
			"run_id", t.RunId,
			"job_id", run.JobID,
			"error", err,
		)
		// Fall back: mark failed without retry.
		s.reconcileMarkFailed(ctx, t.RunId, t.ErrorMessage)
		return
	}

	// Determine whether another attempt is allowed.
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	if run.Attempt < maxAttempts {
		// Schedule retry via side table so the job_runs UPDATE stays HOT.
		retryAt := time.Now().Add(retryBackoffDuration(run.Attempt))
		fields := map[string]any{
			"attempt":     run.Attempt + 1,
			"started_at":  nil,
			"finished_at": nil,
		}
		if t.ErrorMessage != "" {
			fields["error"] = t.ErrorMessage
		}
		if err := s.queries.ScheduleRetry(ctx, t.RunId, retryAt, run.Attempt+1); err != nil {
			slog.Warn("grpc reconcile: schedule retry failed",
				"run_id", t.RunId,
				"error", err,
			)
			return
		}
		if err := s.queries.UpdateRunStatus(ctx, t.RunId, domain.StatusExecuting, domain.StatusQueued, fields); err != nil {
			slog.Warn("grpc reconcile: retry requeue failed",
				"run_id", t.RunId,
				"error", err,
			)
		} else {
			slog.Info("grpc reconcile: run requeued for retry",
				"run_id", t.RunId,
				"attempt", run.Attempt+1,
				"next_retry_at", retryAt,
			)
		}
		return
	}

	// Exhausted retries: mark dead-letter.
	s.reconcileMarkFailed(ctx, t.RunId, t.ErrorMessage)
}

// reconcileMarkFailed transitions a run to StatusDeadLetter.
func (s *workerService) reconcileMarkFailed(ctx context.Context, runID, errMsg string) {
	fields := map[string]any{"finished_at": time.Now()}
	if errMsg != "" {
		fields["error"] = errMsg
	}
	if err := s.queries.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusDeadLetter, fields); err != nil {
		slog.Warn("grpc reconcile: mark failed",
			"run_id", runID,
			"error", err,
		)
	}
}

// validateRegistration enforces the resource bounds and slot-count sanity
// checks on an incoming WorkerRegistration. Returning a typed gRPC status
// error (codes.InvalidArgument) lets the stream handler reject malformed
// registrations without doing any state mutation. Extracted as a pure
// function so it is exhaustively unit-testable.
//
// Slot-count checks defend the dispatcher: PickWorkerForQueue ranks
// workers by SlotsAvailable, so a worker advertising an oversized or
// negative slot count could either monopolize dispatch or wedge it. We
// trust the worker to report its own concurrency, but only within bounds
// the server can enforce.
func validateRegistration(reg *workerv1.WorkerRegistration) error {
	if reg == nil {
		return status.Error(codes.InvalidArgument, "registration must not be nil")
	}
	if strings.TrimSpace(reg.WorkerId) == "" {
		return status.Error(codes.InvalidArgument, "worker_id must be non-empty")
	}
	if len(reg.WorkerId) > maxWorkerIDLen {
		return status.Errorf(codes.InvalidArgument, "worker_id exceeds %d bytes", maxWorkerIDLen)
	}
	if len(reg.Queues) > maxQueuesPerWorker {
		return status.Errorf(codes.InvalidArgument, "too many queues: max %d", maxQueuesPerWorker)
	}
	for _, queue := range reg.Queues {
		if strings.TrimSpace(queue) == "" {
			return status.Error(codes.InvalidArgument, "queue must be non-empty")
		}
		if len(queue) > maxQueueNameBytes {
			return status.Errorf(codes.InvalidArgument, "queue exceeds %d bytes", maxQueueNameBytes)
		}
	}
	if len(reg.JobSlugs) > maxJobSlugsPerWorker {
		return status.Errorf(codes.InvalidArgument, "too many job_slugs: max %d", maxJobSlugsPerWorker)
	}
	for _, slug := range reg.JobSlugs {
		if strings.TrimSpace(slug) == "" {
			return status.Error(codes.InvalidArgument, "job_slug must be non-empty")
		}
		if len(slug) > maxJobSlugBytes {
			return status.Errorf(codes.InvalidArgument, "job_slug exceeds %d bytes", maxJobSlugBytes)
		}
	}
	if len(reg.Metadata) > maxRegistrationMetadataEntries {
		return status.Errorf(codes.InvalidArgument, "too many metadata entries: max %d", maxRegistrationMetadataEntries)
	}
	for key, value := range reg.Metadata {
		if strings.TrimSpace(key) == "" {
			return status.Error(codes.InvalidArgument, "metadata key must be non-empty")
		}
		if len(key) > maxRegistrationMetadataKeyBytes {
			return status.Errorf(codes.InvalidArgument, "metadata key exceeds %d bytes", maxRegistrationMetadataKeyBytes)
		}
		if len(value) > maxRegistrationMetadataValueBytes {
			return status.Errorf(codes.InvalidArgument, "metadata value exceeds %d bytes", maxRegistrationMetadataValueBytes)
		}
	}
	if len(reg.InFlightTasks) > maxInFlightTasks {
		return status.Errorf(codes.InvalidArgument, "too many in-flight tasks: max %d", maxInFlightTasks)
	}
	if reg.SlotsTotal < 0 {
		return status.Errorf(codes.InvalidArgument, "slots_total must be non-negative, got %d", reg.SlotsTotal)
	}
	if reg.SlotsTotal > maxSlotsPerWorker {
		return status.Errorf(codes.InvalidArgument, "slots_total exceeds %d", maxSlotsPerWorker)
	}
	if reg.SlotsAvailable < 0 {
		return status.Errorf(codes.InvalidArgument, "slots_available must be non-negative, got %d", reg.SlotsAvailable)
	}
	if reg.SlotsAvailable > reg.SlotsTotal {
		return status.Errorf(codes.InvalidArgument, "slots_available (%d) exceeds slots_total (%d)", reg.SlotsAvailable, reg.SlotsTotal)
	}
	if len(reg.Hostname) > maxHostnameBytes {
		return status.Errorf(codes.InvalidArgument, "hostname exceeds %d bytes", maxHostnameBytes)
	}
	if len(reg.SdkVersion) > maxSDKVersionBytes {
		return status.Errorf(codes.InvalidArgument, "sdk_version exceeds %d bytes", maxSDKVersionBytes)
	}
	if len(reg.SdkLanguage) > maxSDKLanguageBytes {
		return status.Errorf(codes.InvalidArgument, "sdk_language exceeds %d bytes", maxSDKLanguageBytes)
	}
	if len(reg.Name) > maxNameBytes {
		return status.Errorf(codes.InvalidArgument, "name exceeds %d bytes", maxNameBytes)
	}
	return nil
}

// retryBackoffDuration returns an exponential backoff delay for a given attempt
// (1-indexed). Matches the default policy in internal/worker/backoff.go:
// delay = min(2^(attempt-1), 3600) seconds.
func retryBackoffDuration(attempt int) time.Duration {
	secs := min(1<<(attempt-1), 3600) // 2^(attempt-1), capped at 3600
	return time.Duration(secs) * time.Second
}

// dbUpsertWorker immediately upserts the worker into the DB on registration,
// without waiting for the next dbSync tick.
func (s *workerService) dbUpsertWorker(ctx context.Context, cw *ConnectedWorker) {
	queueName := ""
	if len(cw.Queues) > 0 {
		queueName = cw.Queues[0]
	}
	dw := &domain.Worker{
		ID:        cw.WorkerID,
		ProjectID: cw.ProjectID,
		QueueName: queueName,
		Hostname:  cw.Hostname,
		Version:   cw.SDKVersion,
		Status:    domain.WorkerStatusActive,
	}
	if err := s.queries.RegisterWorker(ctx, dw); err != nil {
		slog.Warn("grpc: immediate db upsert on registration failed",
			"worker_id", cw.WorkerID,
			"error", err,
		)
		return
	}
	if err := s.queries.RenewWorkerStreamLease(ctx, cw.WorkerID, cw.ProjectID, time.Now().Add(s.workerConnectionLease())); err != nil {
		slog.Warn("grpc: initial worker stream lease failed",
			"worker_id", cw.WorkerID,
			"project_id", cw.ProjectID,
			"error", err,
		)
	}
}

// finalizeDisconnect runs the post-stream cleanup writes: mark the worker
// offline in the workers table, then emit the disconnect audit event.
//
// The stream's ctx is cancelled by the time the deferred cleanup fires (that
// cancellation is precisely how we exit), so any DB call using it would fail
// with context canceled. We allocate a fresh background context with a short
// timeout so the offline transition and audit row still land — without this,
// the workers row stays in `active` forever after a clean disconnect and the
// audit log is missing the disconnect event entirely.
func (s *workerService) finalizeDisconnect(projectID, workerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason := "worker disconnected before reporting result"
	openRunIDs := s.openWorkerTaskRunIDs(ctx, workerID, projectID)
	if count, err := s.queries.RequeueOpenWorkerTasks(ctx, workerID, projectID, reason); err != nil {
		slog.Warn("grpc worker disconnect: failed to requeue open tasks",
			"worker_id", workerID,
			"project_id", projectID,
			"error", err,
		)
	} else if count > 0 {
		slog.Info("grpc worker disconnect: requeued open tasks",
			"worker_id", workerID,
			"project_id", projectID,
			"task_count", count,
		)
		s.enqueueRecoveredWorkerRuns(ctx, openRunIDs)
	}
	if err := s.queries.SetWorkerStatus(ctx, workerID, projectID, domain.WorkerStatusOffline); err != nil {
		slog.Warn("grpc worker disconnect: failed to mark offline",
			"worker_id", workerID, "error", err)
	}
	ackChannel := workerDisconnectAckChannel(projectID, workerID)
	if err := s.pub.Publish(ctx, ackChannel, []byte(workerID)); err != nil {
		slog.Warn("grpc worker disconnect: failed to publish ack",
			"worker_id", workerID,
			"project_id", projectID,
			"error", err,
		)
	}
	s.emitWorkerAudit(ctx, domain.AuditActionWorkerDisconnected, projectID, workerID, map[string]any{
		"worker_id": workerID,
	})
	slog.Info("grpc worker disconnected", "worker_id", workerID, "project_id", projectID)
}

func (s *workerService) openWorkerTaskRunIDs(ctx context.Context, workerID, projectID string) []string {
	if s == nil || s.queries == nil {
		return nil
	}

	seen := make(map[string]struct{}, 8)
	runIDs := make([]string, 0, 8)
	for _, taskStatus := range []domain.WorkerTaskStatus{domain.WorkerTaskStatusAssigned, domain.WorkerTaskStatusAccepted} {
		tasks, err := s.queries.ListWorkerTasksByWorker(ctx, workerID, projectID, taskStatus, 10000, 0)
		if err != nil {
			slog.Warn("grpc worker disconnect: failed to list open tasks",
				"worker_id", workerID,
				"project_id", projectID,
				"status", taskStatus,
				"error", err,
			)
			continue
		}
		for _, task := range tasks {
			if task.RunID == "" {
				continue
			}
			if _, ok := seen[task.RunID]; ok {
				continue
			}
			seen[task.RunID] = struct{}{}
			runIDs = append(runIDs, task.RunID)
		}
	}
	return runIDs
}

func (s *workerService) enqueueRecoveredWorkerRuns(ctx context.Context, runIDs []string) {
	if s == nil {
		return
	}
	enqueueRecoveredWorkerRuns(ctx, s.queries, s.readyRunQueue, runIDs, "grpc worker disconnect")
}

type recoveredRunLoader interface {
	GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error)
}

func enqueueRecoveredWorkerRuns(ctx context.Context, q recoveredRunLoader, readyRunQueue ReadyRunEnqueuer, runIDs []string, logPrefix string) {
	if q == nil || readyRunQueue == nil || len(runIDs) == 0 {
		return
	}
	runs, err := q.GetRunsByIDs(ctx, runIDs)
	if err != nil {
		slog.Warn(logPrefix+": failed to load recovered runs",
			"run_count", len(runIDs),
			"error", err,
		)
		return
	}
	for _, runID := range runIDs {
		run := runs[runID]
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		if err := readyRunQueue.EnqueueExisting(ctx, run); err != nil {
			slog.Warn(logPrefix+": failed to enqueue recovered run",
				"run_id", runID,
				"project_id", run.ProjectID,
				"error", err,
			)
		}
	}
}

// emitWorkerAudit writes an audit event for a worker lifecycle transition.
// Failures are logged but do not abort the caller.
func (s *workerService) emitWorkerAudit(ctx context.Context, action, projectID, workerID string, details map[string]any) {
	raw, err := json.Marshal(details)
	if err != nil {
		slog.Warn("grpc audit: marshal details failed", "error", err)
		return
	}
	ev := &domain.AuditEvent{
		ProjectID:    projectID,
		ActorID:      "worker:" + workerID,
		ActorType:    "worker",
		Action:       action,
		ResourceType: "worker",
		ResourceID:   workerID,
		Details:      json.RawMessage(raw),
		CreatedAt:    time.Now(),
	}
	if err := s.queries.CreateAuditEvent(ctx, ev); err != nil {
		slog.Warn("grpc audit: create event failed",
			"action", action,
			"worker_id", workerID,
			"error", err,
		)
	}
}
