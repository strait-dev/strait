package authoring

// StepType identifies the kind of workflow step.
type StepType string

const (
	StepTypeJob          StepType = "job"
	StepTypeApproval     StepType = "approval"
	StepTypeSubWorkflow  StepType = "sub_workflow"
	StepTypeWaitForEvent StepType = "wait_for_event"
	StepTypeSleep        StepType = "sleep"
)

// OnFailureAction defines what happens when a step fails.
type OnFailureAction string

const (
	OnFailureFailWorkflow   OnFailureAction = "fail_workflow"
	OnFailureSkipDependents OnFailureAction = "skip_dependents"
	OnFailureContinue       OnFailureAction = "continue"
)

// ResourceClass defines the resource tier for a step.
type ResourceClass string

const (
	ResourceClassSmall  ResourceClass = "small"
	ResourceClassMedium ResourceClass = "medium"
	ResourceClassLarge  ResourceClass = "large"
)

// RetryBackoff defines the retry backoff strategy.
type RetryBackoff string

const (
	RetryBackoffExponential RetryBackoff = "exponential"
	RetryBackoffFixed       RetryBackoff = "fixed"
)

// BaseStepOptions contains configuration shared by all step types.
type BaseStepOptions struct {
	DependsOn             []string        `json:"-"`
	Condition             map[string]any  `json:"-"`
	OnFailure             OnFailureAction `json:"-"`
	Payload               map[string]any  `json:"-"`
	RetryMaxAttempts      *int            `json:"-"`
	RetryBackoff          RetryBackoff    `json:"-"`
	RetryInitialDelaySecs *int            `json:"-"`
	RetryMaxDelaySecs     *int            `json:"-"`
	TimeoutSecsOverride   *int            `json:"-"`
	OutputTransform       string          `json:"-"`
	ConcurrencyKey        string          `json:"-"`
	ResourceClass         ResourceClass   `json:"-"`
}

// Step is the interface implemented by all workflow step types.
type Step interface {
	StepRef() string
	Type() StepType
	BaseOptions() BaseStepOptions
}

// JobStep executes a registered job.
type JobStep struct {
	Ref   string
	JobID string
	BaseStepOptions
}

func (s *JobStep) StepRef() string              { return s.Ref }
func (s *JobStep) Type() StepType               { return StepTypeJob }
func (s *JobStep) BaseOptions() BaseStepOptions { return s.BaseStepOptions }

// ApprovalStep pauses until manually approved.
type ApprovalStep struct {
	Ref                 string
	ApprovalTimeoutSecs *int
	Approvers           []string
	BaseStepOptions
}

func (s *ApprovalStep) StepRef() string              { return s.Ref }
func (s *ApprovalStep) Type() StepType               { return StepTypeApproval }
func (s *ApprovalStep) BaseOptions() BaseStepOptions { return s.BaseStepOptions }

// SubWorkflowStep triggers a nested workflow.
type SubWorkflowStep struct {
	Ref             string
	SubWorkflowID   string
	MaxNestingDepth *int
	BaseStepOptions
}

func (s *SubWorkflowStep) StepRef() string              { return s.Ref }
func (s *SubWorkflowStep) Type() StepType               { return StepTypeSubWorkflow }
func (s *SubWorkflowStep) BaseOptions() BaseStepOptions { return s.BaseStepOptions }

// WaitForEventStep pauses until an external event is received.
type WaitForEventStep struct {
	Ref              string
	EventKey         string
	EventTimeoutSecs *int
	EventNotifyURL   string
	BaseStepOptions
}

func (s *WaitForEventStep) StepRef() string              { return s.Ref }
func (s *WaitForEventStep) Type() StepType               { return StepTypeWaitForEvent }
func (s *WaitForEventStep) BaseOptions() BaseStepOptions { return s.BaseStepOptions }

// SleepStep pauses for a fixed duration.
type SleepStep struct {
	Ref               string
	SleepDurationSecs int
	BaseStepOptions
}

func (s *SleepStep) StepRef() string              { return s.Ref }
func (s *SleepStep) Type() StepType               { return StepTypeSleep }
func (s *SleepStep) BaseOptions() BaseStepOptions { return s.BaseStepOptions }

// Job creates a job step.
func Job(ref, jobID string, opts ...func(*BaseStepOptions)) *JobStep {
	s := &JobStep{Ref: ref, JobID: jobID}
	for _, opt := range opts {
		opt(&s.BaseStepOptions)
	}
	return s
}

// Approval creates an approval step.
func Approval(ref string, opts ...func(*ApprovalStep)) *ApprovalStep {
	s := &ApprovalStep{Ref: ref}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SubWorkflow creates a sub-workflow step.
func SubWorkflow(ref, workflowID string, opts ...func(*SubWorkflowStep)) *SubWorkflowStep {
	s := &SubWorkflowStep{Ref: ref, SubWorkflowID: workflowID}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WaitForEvent creates a wait-for-event step.
func WaitForEvent(ref, eventKey string, opts ...func(*WaitForEventStep)) *WaitForEventStep {
	s := &WaitForEventStep{Ref: ref, EventKey: eventKey}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Sleep creates a sleep step.
func Sleep(ref string, durationSecs int, opts ...func(*BaseStepOptions)) *SleepStep {
	s := &SleepStep{Ref: ref, SleepDurationSecs: durationSecs}
	for _, opt := range opts {
		opt(&s.BaseStepOptions)
	}
	return s
}

// AI creates a job step with LLM-tuned defaults: 600s timeout, 5 retries,
// exponential backoff, and large resource class.
func AI(ref string, jobID string, opts ...func(*BaseStepOptions)) *JobStep {
	timeout := 600
	retryAttempts := 5
	retryDelay := 2
	retryMax := 120
	s := &JobStep{
		Ref:   ref,
		JobID: jobID,
		BaseStepOptions: BaseStepOptions{
			TimeoutSecsOverride:   &timeout,
			RetryMaxAttempts:      &retryAttempts,
			RetryBackoff:          RetryBackoffExponential,
			RetryInitialDelaySecs: &retryDelay,
			RetryMaxDelaySecs:     &retryMax,
			ResourceClass:         ResourceClassLarge,
		},
	}
	for _, opt := range opts {
		opt(&s.BaseStepOptions)
	}
	return s
}

// DependsOn is an option setter for step dependencies.
func DependsOn(deps ...string) func(*BaseStepOptions) {
	return func(o *BaseStepOptions) {
		o.DependsOn = deps
	}
}

// StepToAPI converts a Step to the snake_case API format.
func StepToAPI(step Step) map[string]any {
	out := map[string]any{
		"step_ref": step.StepRef(),
		"type":     string(step.Type()),
	}

	base := step.BaseOptions()
	if len(base.DependsOn) > 0 {
		out["depends_on"] = base.DependsOn
	}
	if base.Condition != nil {
		out["condition"] = base.Condition
	}
	if base.OnFailure != "" {
		out["on_failure"] = string(base.OnFailure)
	}
	if base.Payload != nil {
		out["payload"] = base.Payload
	}
	if base.RetryMaxAttempts != nil {
		out["retry_max_attempts"] = *base.RetryMaxAttempts
	}
	if base.RetryBackoff != "" {
		out["retry_backoff"] = string(base.RetryBackoff)
	}
	if base.RetryInitialDelaySecs != nil {
		out["retry_initial_delay_secs"] = *base.RetryInitialDelaySecs
	}
	if base.RetryMaxDelaySecs != nil {
		out["retry_max_delay_secs"] = *base.RetryMaxDelaySecs
	}
	if base.TimeoutSecsOverride != nil {
		out["timeout_secs_override"] = *base.TimeoutSecsOverride
	}
	if base.OutputTransform != "" {
		out["output_transform"] = base.OutputTransform
	}
	if base.ConcurrencyKey != "" {
		out["concurrency_key"] = base.ConcurrencyKey
	}
	if base.ResourceClass != "" {
		out["resource_class"] = string(base.ResourceClass)
	}

	switch s := step.(type) {
	case *JobStep:
		out["job_id"] = s.JobID
	case *ApprovalStep:
		if s.ApprovalTimeoutSecs != nil {
			out["approval_timeout_secs"] = *s.ApprovalTimeoutSecs
		}
		if len(s.Approvers) > 0 {
			out["approvers"] = s.Approvers
		}
	case *SubWorkflowStep:
		out["sub_workflow_id"] = s.SubWorkflowID
		if s.MaxNestingDepth != nil {
			out["max_nesting_depth"] = *s.MaxNestingDepth
		}
	case *WaitForEventStep:
		out["event_key"] = s.EventKey
		if s.EventTimeoutSecs != nil {
			out["event_timeout_secs"] = *s.EventTimeoutSecs
		}
		if s.EventNotifyURL != "" {
			out["event_notify_url"] = s.EventNotifyURL
		}
	case *SleepStep:
		out["sleep_duration_secs"] = s.SleepDurationSecs
	}

	return out
}
