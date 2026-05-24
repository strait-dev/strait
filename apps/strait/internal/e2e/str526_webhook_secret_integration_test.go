//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	grpcpkg "strait/internal/api/grpc"
	workerv1 "strait/internal/api/grpc/proto/workerv1"
	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/worker"
)

func TestSTR526CreateJobWebhookSecretPersistsEncryptedSigningSecret(t *testing.T) {
	mustClean(t)

	projectID := "proj-str526-create-" + newID()
	secret := "integration-serve-secret-32-bytes!!"
	body := fmt.Sprintf(`{
		"project_id": %q,
		"name": "STR526 Create",
		"slug": %q,
		"endpoint_url": "https://example.com/hook",
		"webhook_secret": %q
	}`, projectID, "str526-create-"+newID(), secret)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", body, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)
	jobID := asString(t, resp, "id")

	requirePersistedEndpointSigningSecret(t, jobID, secret)
	requireCreateResponseDoesNotLeakSigningSecret(t, resp)
}

func TestSTR526CreateJobWebhookSecretWinsOverEndpointSigningSecret(t *testing.T) {
	mustClean(t)

	projectID := "proj-str526-conflict-" + newID()
	webhookSecret := "sdk-supplied-secret-32-bytes-ok"
	endpointSecret := "ignored-endpoint-secret-32-bytes"
	body := fmt.Sprintf(`{
		"project_id": %q,
		"name": "STR526 Conflict",
		"slug": %q,
		"endpoint_url": "https://example.com/hook",
		"endpoint_signing_secret": %q,
		"webhook_secret": %q
	}`, projectID, "str526-conflict-"+newID(), endpointSecret, webhookSecret)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", body, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)

	requirePersistedEndpointSigningSecret(t, asString(t, resp, "id"), webhookSecret)
}

func TestSTR526BatchCreateJobsPersistsWebhookSecretAlias(t *testing.T) {
	mustClean(t)

	projectID := "proj-str526-batch-" + newID()
	endpointSecret := "batch-endpoint-secret-32-bytes"
	webhookSecret := "batch-webhook-secret-32-bytes-ok"
	endpointSlug := "str526-batch-endpoint-" + newID()
	webhookSlug := "str526-batch-webhook-" + newID()
	body := fmt.Sprintf(`{"jobs":[
		{
			"project_id": %q,
			"name": "STR526 Batch Endpoint",
			"slug": %q,
			"endpoint_url": "https://example.com/endpoint",
			"endpoint_signing_secret": %q
		},
		{
			"project_id": %q,
			"name": "STR526 Batch Webhook",
			"slug": %q,
			"endpoint_url": "https://example.com/webhook",
			"endpoint_signing_secret": "ignored-batch-endpoint-secret-32",
			"webhook_secret": %q
		}
	]}`, projectID, endpointSlug, endpointSecret, projectID, webhookSlug, webhookSecret)

	w := doRequest(t, http.MethodPost, "/v1/jobs/batch", body, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("batch create status = %d, body = %s", w.Code, w.Body.String())
	}

	endpointJob, err := testStore.GetJobBySlug(context.Background(), projectID, endpointSlug)
	if err != nil {
		t.Fatalf("get endpoint job by slug: %v", err)
	}
	webhookJob, err := testStore.GetJobBySlug(context.Background(), projectID, webhookSlug)
	if err != nil {
		t.Fatalf("get webhook job by slug: %v", err)
	}

	requirePersistedEndpointSigningSecret(t, endpointJob.ID, endpointSecret)
	requirePersistedEndpointSigningSecret(t, webhookJob.ID, webhookSecret)
}

func TestSTR526UpdateJobWebhookSecretRotatesPersistedSigningSecret(t *testing.T) {
	mustClean(t)

	projectID := "proj-str526-update-" + newID()
	initialSecret := "initial-endpoint-secret-32-bytes"
	rotatedSecret := "rotated-webhook-secret-32-bytes"
	body := fmt.Sprintf(`{
		"project_id": %q,
		"name": "STR526 Update",
		"slug": %q,
		"endpoint_url": "https://example.com/hook",
		"endpoint_signing_secret": %q
	}`, projectID, "str526-update-"+newID(), initialSecret)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", body, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	patchBody := fmt.Sprintf(`{"webhook_secret": %q}`, rotatedSecret)
	w = doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", patchBody, projectID)
	if w.Code != http.StatusOK {
		t.Fatalf("update job status = %d, body = %s", w.Code, w.Body.String())
	}

	requirePersistedEndpointSigningSecret(t, jobID, rotatedSecret)
}

func TestSTR526HTTPExecutorSignsDispatchCreatedWithWebhookSecret(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-str526-http-dispatch-" + newID()
	secret := "http-dispatch-secret-32-bytes-ok"
	payload := `{"source":"str526","mode":"http"}`

	received := make(chan *http.Request, 1)
	sdkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if verifyHMACSignature(secret, r) {
			received <- r.Clone(context.Background())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(sdkServer.Close)

	createBody := fmt.Sprintf(`{
		"project_id": %q,
		"name": "STR526 HTTP Dispatch",
		"slug": %q,
		"endpoint_url": "https://example.com/placeholder",
		"webhook_secret": %q
	}`, projectID, "str526-http-dispatch-"+newID(), secret)
	w := doRequest(t, http.MethodPost, "/v1/jobs/", createBody, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	if _, err := testEnv.DB.Pool.Exec(ctx, `UPDATE jobs SET endpoint_url = $1 WHERE id = $2`, sdkServer.URL, jobID); err != nil {
		t.Fatalf("replace endpoint_url with test server: %v", err)
	}

	triggerBody := fmt.Sprintf(`{"payload": %s}`, payload)
	w = doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", triggerBody, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger job status = %d, body = %s", w.Code, w.Body.String())
	}
	runID := asString(t, mustDecodeObject(t, w), "id")

	execCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	pool := worker.NewPool(1)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                  pool,
		Queue:                 queue.NewPostgresQueue(testEnv.DB.Pool),
		Wake:                  make(chan struct{}, 1),
		Store:                 store.New(testEnv.DB.Pool),
		TxPool:                testEnv.DB.Pool,
		PollInterval:          25 * time.Millisecond,
		HeartbeatInterval:     time.Hour,
		MaxDequeueBatchSize:   1,
		AllowPrivateEndpoints: true,
		SecretDecryptor:       newSTR526Encryptor(t),
	})
	t.Cleanup(exec.CloseCache)
	go exec.Run(execCtx)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := exec.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown executor: %v", err)
		}
	})

	select {
	case req := <-received:
		if req.Header.Get("X-Strait-Timestamp") == "" {
			t.Fatal("missing X-Strait-Timestamp")
		}
		if sig := req.Header.Get("X-Strait-Signature"); !strings.HasPrefix(sig, "v1=") {
			t.Fatalf("X-Strait-Signature = %q, want v1= prefix", sig)
		}
	case <-time.After(10 * time.Second):
		run, err := testStore.GetRun(ctx, runID)
		if err != nil {
			t.Fatalf("timed out waiting for signed dispatch; get run: %v", err)
		}
		t.Fatalf("timed out waiting for signed dispatch; run status=%s error=%q", run.Status, run.Error)
	}
}

func TestSTR526WorkerAssignmentSignsPayloadCreatedWithWebhookSecret(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-str526-worker-dispatch-" + newID()
	queueName := "str526-worker-" + newID()
	secret := "worker-assignment-secret-32-bytes"
	payload := `{"source":"str526","mode":"worker"}`

	createBody := fmt.Sprintf(`{
		"project_id": %q,
		"name": "STR526 Worker Dispatch",
		"slug": %q,
		"execution_mode": "worker",
		"queue_name": %q,
		"webhook_secret": %q
	}`, projectID, "str526-worker-dispatch-"+newID(), queueName, secret)
	w := doRequest(t, http.MethodPost, "/v1/jobs/", createBody, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create worker job status = %d, body = %s", w.Code, w.Body.String())
	}
	jobID := asString(t, mustDecodeObject(t, w), "id")

	triggerBody := fmt.Sprintf(`{"payload": %s}`, payload)
	w = doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", triggerBody, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger worker job status = %d, body = %s", w.Code, w.Body.String())
	}
	runID := asString(t, mustDecodeObject(t, w), "id")

	workerID := "str526-worker-" + newID()
	sendCh := make(chan *workerv1.ServerMessage, 2)
	registry := grpcpkg.NewConnectionRegistry()
	resultChannels := grpcpkg.NewResultChannelRegistry()
	if err := registry.Register(&grpcpkg.ConnectedWorker{
		WorkerID:       workerID,
		ProjectID:      projectID,
		APIKeyID:       "str526-api-key",
		Queues:         []string{queueName},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         sendCh,
	}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	if _, err := testEnv.DB.Pool.Exec(ctx,
		`INSERT INTO workers (id, project_id, queue_name, hostname, version, status, last_seen_at, registered_at)
		 VALUES ($1, $2, $3, 'test-host', '1.0', 'active', NOW(), NOW())`,
		workerID, projectID, queueName,
	); err != nil {
		t.Fatalf("insert worker row: %v", err)
	}

	dispatcher := grpcpkg.NewWorkerDispatcher(registry, store.New(testEnv.DB.Pool), "test-jwt-key", resultChannels).
		WithSecretDecryptor(newSTR526Encryptor(t))

	dispatchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var dispatchWG sync.WaitGroup
	dispatchWG.Add(1)
	go func() {
		defer dispatchWG.Done()
		run, err := testStore.GetRun(ctx, runID)
		if err != nil {
			t.Errorf("get run: %v", err)
			return
		}
		job, err := testStore.GetJob(ctx, jobID)
		if err != nil {
			t.Errorf("get job: %v", err)
			return
		}
		result, err := dispatcher.WorkerDispatch(dispatchCtx, run, job)
		if err != nil {
			t.Errorf("worker dispatch: %v", err)
			return
		}
		if err := dispatcher.CompleteWorkerTask(ctx, result, domain.WorkerTaskStatusCompleted); err != nil {
			t.Errorf("complete worker task: %v", err)
		}
	}()

	select {
	case msg := <-sendCh:
		assignmentMsg, ok := msg.Payload.(*workerv1.ServerMessage_TaskAssignment)
		if !ok {
			t.Fatalf("message payload = %T, want task assignment", msg.Payload)
		}
		assignment := assignmentMsg.TaskAssignment
		if assignment.HmacTimestamp == "" {
			t.Fatal("missing hmac_timestamp")
		}
		if assignment.HmacSignature == "" {
			t.Fatal("missing hmac_signature")
		}
		if got, want := assignment.HmacSignature, worker.SignHTTPDispatch(secret, assignment.HmacTimestamp, assignment.PayloadJson); got != want {
			t.Fatalf("hmac_signature = %q, want %q", got, want)
		}
		resultChannels.Send(runID, projectID, workerID, &workerv1.TaskResult{
			RunId:        runID,
			Status:       "success",
			AssignmentId: assignment.AssignmentId,
			Attempt:      assignment.Attempt,
		})
	case <-dispatchCtx.Done():
		t.Fatal("timed out waiting for task assignment")
	}
	dispatchWG.Wait()
}

func requirePersistedEndpointSigningSecret(t *testing.T, jobID, want string) {
	t.Helper()

	job, err := testStore.GetJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if job.EndpointSigningSecret == "" {
		t.Fatal("endpoint_signing_secret was not persisted")
	}
	if job.EndpointSigningSecret == want {
		t.Fatal("endpoint_signing_secret was persisted in plaintext")
	}
	if !straitcrypto.IsEncryptedField(job.EndpointSigningSecret) {
		t.Fatalf("endpoint_signing_secret = %q, want encrypted field", job.EndpointSigningSecret)
	}

	got, err := straitcrypto.DecryptField(newSTR526Encryptor(t), job.EndpointSigningSecret)
	if err != nil {
		t.Fatalf("decrypt endpoint_signing_secret: %v", err)
	}
	if got != want {
		t.Fatalf("decrypted endpoint_signing_secret = %q, want %q", got, want)
	}
	if job.WebhookSecret != "" {
		t.Fatalf("legacy webhook_secret column = %q, want empty; dispatch signing must use endpoint_signing_secret", job.WebhookSecret)
	}
}

func requireCreateResponseDoesNotLeakSigningSecret(t *testing.T, resp map[string]any) {
	t.Helper()
	for _, key := range []string{"endpoint_signing_secret", "webhook_secret"} {
		if _, ok := resp[key]; ok {
			t.Fatalf("create response leaked %s: %#v", key, resp[key])
		}
	}
}

func newSTR526Encryptor(t *testing.T) *straitcrypto.KeyRotator {
	t.Helper()
	enc, err := straitcrypto.NewKeyRotatorFromStrings(testEncryptionKey)
	if err != nil {
		t.Fatalf("new key rotator: %v", err)
	}
	return enc
}
