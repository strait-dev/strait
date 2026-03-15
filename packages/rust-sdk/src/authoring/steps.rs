use serde_json::{json, Value};

pub const STEP_TYPE_JOB: &str = "job";
pub const STEP_TYPE_APPROVAL: &str = "approval";
pub const STEP_TYPE_SUB_WORKFLOW: &str = "sub_workflow";
pub const STEP_TYPE_WAIT_FOR_EVENT: &str = "wait_for_event";
pub const STEP_TYPE_SLEEP: &str = "sleep";

pub const ON_FAILURE_FAIL_WORKFLOW: &str = "fail_workflow";
pub const ON_FAILURE_SKIP_DEPENDENTS: &str = "skip_dependents";
pub const ON_FAILURE_CONTINUE: &str = "continue";

pub const RESOURCE_CLASS_SMALL: &str = "small";
pub const RESOURCE_CLASS_MEDIUM: &str = "medium";
pub const RESOURCE_CLASS_LARGE: &str = "large";

pub const RETRY_BACKOFF_EXPONENTIAL: &str = "exponential";
pub const RETRY_BACKOFF_FIXED: &str = "fixed";

#[derive(Debug, Clone, Default)]
pub struct BaseStepOptions {
    pub depends_on: Vec<String>,
    pub condition: Option<String>,
    pub on_failure: Option<String>,
    pub payload: Option<Value>,
    pub retry_max_attempts: Option<u32>,
    pub retry_backoff: Option<String>,
    pub retry_initial_delay_secs: Option<u32>,
    pub retry_max_delay_secs: Option<u32>,
    pub timeout_secs_override: Option<u32>,
    pub output_transform: Option<String>,
    pub concurrency_key: Option<String>,
    pub resource_class: Option<String>,
}

#[derive(Debug, Clone)]
pub enum Step {
    Job {
        ref_name: String,
        job_id: String,
        options: BaseStepOptions,
    },
    Approval {
        ref_name: String,
        approval_timeout_secs: Option<u32>,
        approvers: Option<Vec<String>>,
        options: BaseStepOptions,
    },
    SubWorkflow {
        ref_name: String,
        sub_workflow_id: String,
        max_nesting_depth: Option<u32>,
        options: BaseStepOptions,
    },
    WaitForEvent {
        ref_name: String,
        event_key: String,
        event_timeout_secs: Option<u32>,
        event_notify_url: Option<String>,
        options: BaseStepOptions,
    },
    Sleep {
        ref_name: String,
        sleep_duration_secs: u32,
        options: BaseStepOptions,
    },
}

impl Step {
    pub fn step_ref(&self) -> &str {
        match self {
            Step::Job { ref_name, .. } => ref_name,
            Step::Approval { ref_name, .. } => ref_name,
            Step::SubWorkflow { ref_name, .. } => ref_name,
            Step::WaitForEvent { ref_name, .. } => ref_name,
            Step::Sleep { ref_name, .. } => ref_name,
        }
    }

    pub fn step_type(&self) -> &str {
        match self {
            Step::Job { .. } => STEP_TYPE_JOB,
            Step::Approval { .. } => STEP_TYPE_APPROVAL,
            Step::SubWorkflow { .. } => STEP_TYPE_SUB_WORKFLOW,
            Step::WaitForEvent { .. } => STEP_TYPE_WAIT_FOR_EVENT,
            Step::Sleep { .. } => STEP_TYPE_SLEEP,
        }
    }

    pub fn depends_on(&self) -> &[String] {
        match self {
            Step::Job { options, .. } => &options.depends_on,
            Step::Approval { options, .. } => &options.depends_on,
            Step::SubWorkflow { options, .. } => &options.depends_on,
            Step::WaitForEvent { options, .. } => &options.depends_on,
            Step::Sleep { options, .. } => &options.depends_on,
        }
    }

    pub fn to_api(&self) -> Value {
        match self {
            Step::Job {
                ref_name,
                job_id,
                options,
            } => {
                let mut h = json!({ "ref": ref_name, "type": STEP_TYPE_JOB, "job_id": job_id });
                add_base_options(&mut h, options);
                h
            }
            Step::Approval {
                ref_name,
                approval_timeout_secs,
                approvers,
                options,
            } => {
                let mut h = json!({ "ref": ref_name, "type": STEP_TYPE_APPROVAL });
                if let Some(t) = approval_timeout_secs {
                    h["approval_timeout_secs"] = json!(t);
                }
                if let Some(a) = approvers {
                    h["approvers"] = json!(a);
                }
                add_base_options(&mut h, options);
                h
            }
            Step::SubWorkflow {
                ref_name,
                sub_workflow_id,
                max_nesting_depth,
                options,
            } => {
                let mut h = json!({ "ref": ref_name, "type": STEP_TYPE_SUB_WORKFLOW, "sub_workflow_id": sub_workflow_id });
                if let Some(d) = max_nesting_depth {
                    h["max_nesting_depth"] = json!(d);
                }
                add_base_options(&mut h, options);
                h
            }
            Step::WaitForEvent {
                ref_name,
                event_key,
                event_timeout_secs,
                event_notify_url,
                options,
            } => {
                let mut h = json!({ "ref": ref_name, "type": STEP_TYPE_WAIT_FOR_EVENT, "event_key": event_key });
                if let Some(t) = event_timeout_secs {
                    h["event_timeout_secs"] = json!(t);
                }
                if let Some(u) = event_notify_url {
                    h["event_notify_url"] = json!(u);
                }
                add_base_options(&mut h, options);
                h
            }
            Step::Sleep {
                ref_name,
                sleep_duration_secs,
                options,
            } => {
                let mut h = json!({ "ref": ref_name, "type": STEP_TYPE_SLEEP, "sleep_duration_secs": sleep_duration_secs });
                add_base_options(&mut h, options);
                h
            }
        }
    }
}

fn add_base_options(h: &mut Value, opts: &BaseStepOptions) {
    if !opts.depends_on.is_empty() {
        h["depends_on"] = json!(opts.depends_on);
    }
    if let Some(c) = &opts.condition {
        h["condition"] = json!(c);
    }
    if let Some(f) = &opts.on_failure {
        h["on_failure"] = json!(f);
    }
    if let Some(p) = &opts.payload {
        h["payload"] = p.clone();
    }
    if let Some(r) = opts.retry_max_attempts {
        h["retry_max_attempts"] = json!(r);
    }
    if let Some(b) = &opts.retry_backoff {
        h["retry_backoff"] = json!(b);
    }
    if let Some(d) = opts.retry_initial_delay_secs {
        h["retry_initial_delay_secs"] = json!(d);
    }
    if let Some(d) = opts.retry_max_delay_secs {
        h["retry_max_delay_secs"] = json!(d);
    }
    if let Some(t) = opts.timeout_secs_override {
        h["timeout_secs_override"] = json!(t);
    }
    if let Some(o) = &opts.output_transform {
        h["output_transform"] = json!(o);
    }
    if let Some(k) = &opts.concurrency_key {
        h["concurrency_key"] = json!(k);
    }
    if let Some(r) = &opts.resource_class {
        h["resource_class"] = json!(r);
    }
}

// Builder functions
pub fn job_step(
    ref_name: impl Into<String>,
    job_id: impl Into<String>,
    options: BaseStepOptions,
) -> Step {
    Step::Job {
        ref_name: ref_name.into(),
        job_id: job_id.into(),
        options,
    }
}

pub fn approval_step(ref_name: impl Into<String>, options: BaseStepOptions) -> Step {
    Step::Approval {
        ref_name: ref_name.into(),
        approval_timeout_secs: None,
        approvers: None,
        options,
    }
}

pub fn sub_workflow_step(
    ref_name: impl Into<String>,
    sub_workflow_id: impl Into<String>,
    options: BaseStepOptions,
) -> Step {
    Step::SubWorkflow {
        ref_name: ref_name.into(),
        sub_workflow_id: sub_workflow_id.into(),
        max_nesting_depth: None,
        options,
    }
}

pub fn wait_for_event_step(
    ref_name: impl Into<String>,
    event_key: impl Into<String>,
    options: BaseStepOptions,
) -> Step {
    Step::WaitForEvent {
        ref_name: ref_name.into(),
        event_key: event_key.into(),
        event_timeout_secs: None,
        event_notify_url: None,
        options,
    }
}

pub fn sleep_step(
    ref_name: impl Into<String>,
    duration_secs: u32,
    options: BaseStepOptions,
) -> Step {
    Step::Sleep {
        ref_name: ref_name.into(),
        sleep_duration_secs: duration_secs,
        options,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_job_step_ref() {
        let step = job_step("s1", "j1", BaseStepOptions::default());
        assert_eq!(step.step_ref(), "s1");
    }

    #[test]
    fn test_job_step_type() {
        let step = job_step("s1", "j1", BaseStepOptions::default());
        assert_eq!(step.step_type(), "job");
    }

    #[test]
    fn test_approval_step_type() {
        let step = approval_step("s1", BaseStepOptions::default());
        assert_eq!(step.step_type(), "approval");
    }

    #[test]
    fn test_sub_workflow_step_type() {
        let step = sub_workflow_step("s1", "wf1", BaseStepOptions::default());
        assert_eq!(step.step_type(), "sub_workflow");
    }

    #[test]
    fn test_wait_for_event_step_type() {
        let step = wait_for_event_step("s1", "evt-key", BaseStepOptions::default());
        assert_eq!(step.step_type(), "wait_for_event");
    }

    #[test]
    fn test_sleep_step_type() {
        let step = sleep_step("s1", 30, BaseStepOptions::default());
        assert_eq!(step.step_type(), "sleep");
    }

    #[test]
    fn test_job_step_to_api_basic() {
        let step = job_step("s1", "job-1", BaseStepOptions::default());
        let api = step.to_api();
        assert_eq!(api["ref"], "s1");
        assert_eq!(api["type"], "job");
        assert_eq!(api["job_id"], "job-1");
    }

    #[test]
    fn test_step_with_depends_on() {
        let step = job_step(
            "s2",
            "job-2",
            BaseStepOptions {
                depends_on: vec!["s1".to_string()],
                ..Default::default()
            },
        );
        let api = step.to_api();
        assert_eq!(api["depends_on"], json!(["s1"]));
    }

    #[test]
    fn test_step_depends_on_accessor() {
        let step = job_step(
            "s2",
            "j",
            BaseStepOptions {
                depends_on: vec!["s1".to_string()],
                ..Default::default()
            },
        );
        assert_eq!(step.depends_on(), &["s1".to_string()]);
    }

    #[test]
    fn test_step_empty_depends_on() {
        let step = job_step("s1", "j1", BaseStepOptions::default());
        assert!(step.depends_on().is_empty());
    }

    #[test]
    fn test_sleep_step_to_api() {
        let step = sleep_step("delay", 30, BaseStepOptions::default());
        let api = step.to_api();
        assert_eq!(api["type"], "sleep");
        assert_eq!(api["sleep_duration_secs"], 30);
        assert_eq!(api["ref"], "delay");
    }

    #[test]
    fn test_approval_step_to_api() {
        let step = approval_step("approve", BaseStepOptions::default());
        let api = step.to_api();
        assert_eq!(api["type"], "approval");
        assert_eq!(api["ref"], "approve");
    }

    #[test]
    fn test_sub_workflow_step_to_api() {
        let step = sub_workflow_step("sub", "wf-1", BaseStepOptions::default());
        let api = step.to_api();
        assert_eq!(api["type"], "sub_workflow");
        assert_eq!(api["sub_workflow_id"], "wf-1");
    }

    #[test]
    fn test_wait_for_event_step_to_api() {
        let step = wait_for_event_step("wait", "my-event", BaseStepOptions::default());
        let api = step.to_api();
        assert_eq!(api["type"], "wait_for_event");
        assert_eq!(api["event_key"], "my-event");
    }

    #[test]
    fn test_step_with_retry_options() {
        let opts = BaseStepOptions {
            retry_max_attempts: Some(3),
            retry_backoff: Some("exponential".to_string()),
            retry_initial_delay_secs: Some(1),
            retry_max_delay_secs: Some(60),
            ..Default::default()
        };
        let step = job_step("s1", "j1", opts);
        let api = step.to_api();
        assert_eq!(api["retry_max_attempts"], 3);
        assert_eq!(api["retry_backoff"], "exponential");
        assert_eq!(api["retry_initial_delay_secs"], 1);
        assert_eq!(api["retry_max_delay_secs"], 60);
    }

    #[test]
    fn test_step_with_resource_class() {
        let opts = BaseStepOptions {
            resource_class: Some("large".to_string()),
            ..Default::default()
        };
        let step = job_step("s1", "j1", opts);
        let api = step.to_api();
        assert_eq!(api["resource_class"], "large");
    }

    #[test]
    fn test_step_with_condition() {
        let opts = BaseStepOptions {
            condition: Some("output.status == 'ok'".to_string()),
            ..Default::default()
        };
        let step = job_step("s1", "j1", opts);
        let api = step.to_api();
        assert_eq!(api["condition"], "output.status == 'ok'");
    }

    #[test]
    fn test_step_with_on_failure() {
        let opts = BaseStepOptions {
            on_failure: Some("skip_dependents".to_string()),
            ..Default::default()
        };
        let step = job_step("s1", "j1", opts);
        let api = step.to_api();
        assert_eq!(api["on_failure"], "skip_dependents");
    }

    #[test]
    fn test_step_with_payload() {
        let opts = BaseStepOptions {
            payload: Some(json!({"key": "value"})),
            ..Default::default()
        };
        let step = job_step("s1", "j1", opts);
        let api = step.to_api();
        assert_eq!(api["payload"]["key"], "value");
    }

    #[test]
    fn test_step_with_timeout_override() {
        let opts = BaseStepOptions {
            timeout_secs_override: Some(120),
            ..Default::default()
        };
        let step = job_step("s1", "j1", opts);
        let api = step.to_api();
        assert_eq!(api["timeout_secs_override"], 120);
    }

    #[test]
    fn test_step_omits_none_fields() {
        let step = job_step("s1", "j1", BaseStepOptions::default());
        let api = step.to_api();
        assert!(api.get("depends_on").is_none());
        assert!(api.get("condition").is_none());
        assert!(api.get("on_failure").is_none());
        assert!(api.get("retry_max_attempts").is_none());
        assert!(api.get("resource_class").is_none());
    }

    #[test]
    fn test_step_constants() {
        assert_eq!(STEP_TYPE_JOB, "job");
        assert_eq!(STEP_TYPE_APPROVAL, "approval");
        assert_eq!(STEP_TYPE_SUB_WORKFLOW, "sub_workflow");
        assert_eq!(STEP_TYPE_WAIT_FOR_EVENT, "wait_for_event");
        assert_eq!(STEP_TYPE_SLEEP, "sleep");
    }

    #[test]
    fn test_on_failure_constants() {
        assert_eq!(ON_FAILURE_FAIL_WORKFLOW, "fail_workflow");
        assert_eq!(ON_FAILURE_SKIP_DEPENDENTS, "skip_dependents");
        assert_eq!(ON_FAILURE_CONTINUE, "continue");
    }

    #[test]
    fn test_resource_class_constants() {
        assert_eq!(RESOURCE_CLASS_SMALL, "small");
        assert_eq!(RESOURCE_CLASS_MEDIUM, "medium");
        assert_eq!(RESOURCE_CLASS_LARGE, "large");
    }

    #[test]
    fn test_retry_backoff_constants() {
        assert_eq!(RETRY_BACKOFF_EXPONENTIAL, "exponential");
        assert_eq!(RETRY_BACKOFF_FIXED, "fixed");
    }
}
