// Hand-written message types derived from proto/worker/v1/worker.proto.
// Run `make proto` (requires buf: brew install bufbuild/buf/buf) to regenerate
// this file from the proto source. The buf config lives in buf.yaml / buf.gen.yaml.
// DO NOT add protoc-gen-go generated code markers until buf is wired into CI.
package workerv1

// WorkerMessage is sent from the worker SDK to the server.
type WorkerMessage struct {
	// Payload is the oneof payload.
	Payload isWorkerMessage_Payload
}

type isWorkerMessage_Payload interface{ isWorkerMessage_Payload() }

// WorkerMessage_Registration wraps a WorkerRegistration.
type WorkerMessage_Registration struct{ Registration *WorkerRegistration }

// WorkerMessage_Heartbeat wraps a Heartbeat.
type WorkerMessage_Heartbeat struct{ Heartbeat *Heartbeat }

// WorkerMessage_TaskResult wraps a TaskResult.
type WorkerMessage_TaskResult struct{ TaskResult *TaskResult }

// WorkerMessage_LogLine wraps a LogLine.
type WorkerMessage_LogLine struct{ LogLine *LogLine }

// WorkerMessage_Ack wraps an Acknowledged message.
type WorkerMessage_Ack struct{ Ack *Acknowledged }

func (*WorkerMessage_Registration) isWorkerMessage_Payload() {}
func (*WorkerMessage_Heartbeat) isWorkerMessage_Payload()    {}
func (*WorkerMessage_TaskResult) isWorkerMessage_Payload()   {}
func (*WorkerMessage_LogLine) isWorkerMessage_Payload()      {}
func (*WorkerMessage_Ack) isWorkerMessage_Payload()          {}

// ServerMessage is sent from the server to the worker SDK.
type ServerMessage struct {
	// Payload is the oneof payload.
	Payload isServerMessage_Payload
}

type isServerMessage_Payload interface{ isServerMessage_Payload() }

// ServerMessage_TaskAssignment wraps a TaskAssignment.
type ServerMessage_TaskAssignment struct{ TaskAssignment *TaskAssignment }

// ServerMessage_CancelTask wraps a CancelTask.
type ServerMessage_CancelTask struct{ CancelTask *CancelTask }

// ServerMessage_Ack wraps an Acknowledged message.
type ServerMessage_Ack struct{ Ack *Acknowledged }

func (*ServerMessage_TaskAssignment) isServerMessage_Payload() {}
func (*ServerMessage_CancelTask) isServerMessage_Payload()     {}
func (*ServerMessage_Ack) isServerMessage_Payload()            {}

// WorkerRegistration is the first message a worker sends upon connecting.
type WorkerRegistration struct {
	WorkerID       string            `json:"worker_id"`
	Name           string            `json:"name"`
	Queues         []string          `json:"queues"`
	JobSlugs       []string          `json:"job_slugs"`
	SDKVersion     string            `json:"sdk_version"`
	SDKLanguage    string            `json:"sdk_language"`
	Hostname       string            `json:"hostname"`
	SlotsTotal     int32             `json:"slots_total"`
	SlotsAvailable int32             `json:"slots_available"`
	InFlightTasks  []*InFlightTask   `json:"in_flight_tasks"`
	Metadata       map[string]string `json:"metadata"`
}

// InFlightTask describes a task the worker was executing at the time of (re)connection.
type InFlightTask struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"` // completed | failed | abandoned
	ErrorMessage string `json:"error_message"`
	OutputJSON   []byte `json:"output_json"`
}

// TaskAssignment is sent from the server to a worker to start a run.
type TaskAssignment struct {
	RunID         string            `json:"run_id"`
	JobSlug       string            `json:"job_slug"`
	Queue         string            `json:"queue"`
	PayloadJSON   []byte            `json:"payload_json"`
	TimeoutSecs   int32             `json:"timeout_secs"`
	RunTokenJWT   string            `json:"run_token_jwt"`
	HMACSignature string            `json:"hmac_signature"`
	HMACTimestamp string            `json:"hmac_timestamp"`
	Metadata      map[string]string `json:"metadata"`
}

// TaskResult is sent from the worker when a run finishes.
type TaskResult struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"` // success | failed
	OutputJSON   []byte `json:"output_json"`
	ErrorMessage string `json:"error_message"`
	DurationMS   int64  `json:"duration_ms"`
}

// Heartbeat is sent periodically by the worker.
type Heartbeat struct {
	SlotsAvailable int32 `json:"slots_available"`
	TimestampUnix  int64 `json:"timestamp_unix"`
}

// LogLine is a structured log emitted by a running task.
type LogLine struct {
	RunID           string `json:"run_id"`
	Level           string `json:"level"`
	Message         string `json:"message"`
	TimestampUnixMS int64  `json:"timestamp_unix_ms"`
}

// CancelTask instructs the worker to abort a specific run.
type CancelTask struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

// Acknowledged is a generic ack message.
type Acknowledged struct {
	ID string `json:"id"`
}
