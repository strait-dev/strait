use strait_sdk::authoring::dag_validation::validate_dag;
use strait_sdk::authoring::job::{define_job, JobOptions};
use strait_sdk::authoring::steps::{approval_step, job_step, sleep_step, BaseStepOptions};
use strait_sdk::authoring::workflow::{define_workflow, WorkflowOptions};
use strait_sdk::composition::idempotency::{with_idempotency, with_idempotency_header};
use strait_sdk::composition::result::{from_fn, StraitResult};
use strait_sdk::composition::retry::{JitterStrategy, RetryOptions};
use strait_sdk::config::{
    get_authorization_header, normalize_base_url, AuthMode, AuthType, Config,
};
use strait_sdk::errors::map_http_error;
use strait_sdk::fsm::run::{
    can_transition_run, is_terminal_run_status, transition_run, RunEvent, RunStatus,
};
use strait_sdk::fsm::step::{
    is_terminal_step_run_status, transition_step_run, StepRunEvent, StepRunStatus,
};
use strait_sdk::fsm::workflow::{
    is_terminal_workflow_run_status, transition_workflow_run, WorkflowRunEvent, WorkflowRunStatus,
};
use strait_sdk::http::substitute_path_params;
use strait_sdk::StraitClient;

use std::collections::HashMap;

// ---- Config integration tests ----

#[test]
fn test_config_normalize_and_auth_header() {
    let url = normalize_base_url("https://api.strait.dev/");
    let auth = AuthMode {
        auth_type: AuthType::Bearer,
        token: "sk-test-123".to_string(),
    };
    let header = get_authorization_header(&auth);
    assert_eq!(url, "https://api.strait.dev");
    assert_eq!(header, "Bearer sk-test-123");
}

#[test]
fn test_full_config_construction() {
    let mut headers = HashMap::new();
    headers.insert("X-Request-Id".to_string(), "req-1".to_string());
    let config = Config {
        base_url: normalize_base_url("https://api.example.com/"),
        auth: AuthMode {
            auth_type: AuthType::ApiKey,
            token: "key-abc".to_string(),
        },
        default_headers: headers,
        timeout_ms: 10_000,
    };
    assert_eq!(config.base_url, "https://api.example.com");
    assert_eq!(config.timeout_ms, 10_000);
    assert_eq!(config.default_headers.len(), 1);
}

// ---- Error mapping integration tests ----

#[test]
fn test_error_mapping_comprehensive() {
    let test_cases: Vec<(u16, &str)> = vec![
        (401, "Unauthorized"),
        (403, "Unauthorized"),
        (404, "NotFound"),
        (409, "Conflict"),
        (429, "RateLimited"),
        (500, "Api"),
        (502, "Api"),
        (503, "Api"),
    ];

    for (status, expected_variant) in test_cases {
        let err = map_http_error(status, format!("error {}", status), None);
        let debug = format!("{:?}", err);
        assert!(
            debug.contains(expected_variant),
            "status {} should map to {}, got {:?}",
            status,
            expected_variant,
            err
        );
    }
}

// ---- HTTP path substitution integration tests ----

#[test]
fn test_path_substitution_real_paths() {
    assert_eq!(
        substitute_path_params("/v1/jobs/{jobID}/trigger", &[("jobID", "job-abc-123")]),
        "/v1/jobs/job-abc-123/trigger"
    );
    assert_eq!(
        substitute_path_params(
            "/v1/jobs/{jobID}/versions/{versionID}",
            &[("jobID", "j1"), ("versionID", "v2")]
        ),
        "/v1/jobs/j1/versions/v2"
    );
    assert_eq!(
        substitute_path_params("/v1/runs/{runID}/events", &[("runID", "run-xyz")]),
        "/v1/runs/run-xyz/events"
    );
}

// ---- FSM integration tests ----

#[test]
fn test_run_lifecycle_happy_path() {
    let mut status = RunStatus::Delayed;
    status = transition_run(status, RunEvent::Enqueue).unwrap();
    assert_eq!(status, RunStatus::Queued);
    status = transition_run(status, RunEvent::Dequeue).unwrap();
    assert_eq!(status, RunStatus::Dequeued);
    status = transition_run(status, RunEvent::Execute).unwrap();
    assert_eq!(status, RunStatus::Executing);
    status = transition_run(status, RunEvent::Complete).unwrap();
    assert_eq!(status, RunStatus::Completed);
    assert!(is_terminal_run_status(status));
}

#[test]
fn test_run_lifecycle_failure_and_retry() {
    let mut status = RunStatus::Delayed;
    status = transition_run(status, RunEvent::Enqueue).unwrap();
    status = transition_run(status, RunEvent::Dequeue).unwrap();
    status = transition_run(status, RunEvent::Execute).unwrap();
    status = transition_run(status, RunEvent::Fail).unwrap();
    assert_eq!(status, RunStatus::Failed);
    assert!(is_terminal_run_status(status));
    status = transition_run(status, RunEvent::Requeue).unwrap();
    assert_eq!(status, RunStatus::Queued);
    assert!(!is_terminal_run_status(status));
}

#[test]
fn test_run_lifecycle_failure_to_dead_letter() {
    let mut status = RunStatus::Executing;
    status = transition_run(status, RunEvent::Fail).unwrap();
    status = transition_run(status, RunEvent::DeadLetter).unwrap();
    assert_eq!(status, RunStatus::DeadLetter);
}

#[test]
fn test_run_lifecycle_replay_flow() {
    let mut status = RunStatus::Executing;
    status = transition_run(status, RunEvent::Crash).unwrap();
    assert_eq!(status, RunStatus::Crashed);
    status = transition_run(status, RunEvent::Replay).unwrap();
    assert_eq!(status, RunStatus::ReplayStaged);
    status = transition_run(status, RunEvent::Enqueue).unwrap();
    assert_eq!(status, RunStatus::Queued);
}

#[test]
fn test_run_lifecycle_wait_and_resume() {
    let mut status = RunStatus::Executing;
    status = transition_run(status, RunEvent::Wait).unwrap();
    assert_eq!(status, RunStatus::Waiting);
    status = transition_run(status, RunEvent::Execute).unwrap();
    assert_eq!(status, RunStatus::Executing);
    status = transition_run(status, RunEvent::Complete).unwrap();
    assert_eq!(status, RunStatus::Completed);
}

#[test]
fn test_workflow_lifecycle_happy_path() {
    let mut status = WorkflowRunStatus::Pending;
    status = transition_workflow_run(status, WorkflowRunEvent::Start).unwrap();
    assert_eq!(status, WorkflowRunStatus::Running);
    status = transition_workflow_run(status, WorkflowRunEvent::Complete).unwrap();
    assert_eq!(status, WorkflowRunStatus::Completed);
    assert!(is_terminal_workflow_run_status(status));
}

#[test]
fn test_workflow_lifecycle_pause_resume() {
    let mut status = WorkflowRunStatus::Running;
    status = transition_workflow_run(status, WorkflowRunEvent::Pause).unwrap();
    assert_eq!(status, WorkflowRunStatus::Paused);
    status = transition_workflow_run(status, WorkflowRunEvent::Resume).unwrap();
    assert_eq!(status, WorkflowRunStatus::Running);
    status = transition_workflow_run(status, WorkflowRunEvent::Complete).unwrap();
    assert_eq!(status, WorkflowRunStatus::Completed);
}

#[test]
fn test_step_lifecycle_happy_path() {
    let mut status = StepRunStatus::Pending;
    status = transition_step_run(status, StepRunEvent::Start).unwrap();
    assert_eq!(status, StepRunStatus::Running);
    status = transition_step_run(status, StepRunEvent::Complete).unwrap();
    assert_eq!(status, StepRunStatus::Completed);
    assert!(is_terminal_step_run_status(status));
}

#[test]
fn test_step_lifecycle_wait_then_run() {
    let mut status = StepRunStatus::Pending;
    status = transition_step_run(status, StepRunEvent::Wait).unwrap();
    assert_eq!(status, StepRunStatus::Waiting);
    status = transition_step_run(status, StepRunEvent::Start).unwrap();
    assert_eq!(status, StepRunStatus::Running);
    status = transition_step_run(status, StepRunEvent::Complete).unwrap();
    assert_eq!(status, StepRunStatus::Completed);
}

// ---- DAG validation integration tests ----

#[test]
fn test_complex_dag_with_multiple_step_types() {
    let steps = vec![
        job_step("fetch", "fetch-job", BaseStepOptions::default()),
        sleep_step(
            "pause",
            10,
            BaseStepOptions {
                depends_on: vec!["fetch".to_string()],
                ..Default::default()
            },
        ),
        approval_step(
            "approve",
            BaseStepOptions {
                depends_on: vec!["pause".to_string()],
                ..Default::default()
            },
        ),
        job_step(
            "deploy",
            "deploy-job",
            BaseStepOptions {
                depends_on: vec!["approve".to_string()],
                ..Default::default()
            },
        ),
    ];
    let sorted = validate_dag(&steps).unwrap();
    assert_eq!(sorted, vec!["fetch", "pause", "approve", "deploy"]);
}

#[test]
fn test_dag_with_fan_out_fan_in() {
    let steps = vec![
        job_step("start", "j1", BaseStepOptions::default()),
        job_step(
            "branch-a",
            "j2",
            BaseStepOptions {
                depends_on: vec!["start".to_string()],
                ..Default::default()
            },
        ),
        job_step(
            "branch-b",
            "j3",
            BaseStepOptions {
                depends_on: vec!["start".to_string()],
                ..Default::default()
            },
        ),
        job_step(
            "branch-c",
            "j4",
            BaseStepOptions {
                depends_on: vec!["start".to_string()],
                ..Default::default()
            },
        ),
        job_step(
            "join",
            "j5",
            BaseStepOptions {
                depends_on: vec![
                    "branch-a".to_string(),
                    "branch-b".to_string(),
                    "branch-c".to_string(),
                ],
                ..Default::default()
            },
        ),
    ];
    let sorted = validate_dag(&steps).unwrap();
    let start_idx = sorted.iter().position(|s| s == "start").unwrap();
    let join_idx = sorted.iter().position(|s| s == "join").unwrap();
    assert!(start_idx < join_idx);
}

// ---- Job & Workflow definition integration tests ----

#[test]
fn test_job_define_and_register() {
    let job = define_job(JobOptions {
        name: Some("ETL Pipeline".to_string()),
        slug: Some("etl-pipeline".to_string()),
        endpoint_url: Some("https://example.com/etl".to_string()),
        cron: Some("0 2 * * *".to_string()),
        max_attempts: Some(3),
        timeout_secs: Some(300),
        ..Default::default()
    });
    let body = job.to_registration_body(Some("proj-123"));
    assert_eq!(body["name"], "ETL Pipeline");
    assert_eq!(body["slug"], "etl-pipeline");
    assert_eq!(body["project_id"], "proj-123");
    assert_eq!(body["cron"], "0 2 * * *");
    assert_eq!(body["max_attempts"], 3);
    assert_eq!(body["timeout_secs"], 300);
}

#[test]
fn test_workflow_define_with_valid_steps() {
    let steps = vec![
        job_step("s1", "j1", BaseStepOptions::default()),
        job_step(
            "s2",
            "j2",
            BaseStepOptions {
                depends_on: vec!["s1".to_string()],
                ..Default::default()
            },
        ),
    ];
    let wf = define_workflow(WorkflowOptions {
        name: Some("My Workflow".to_string()),
        slug: Some("my-wf".to_string()),
        steps: Some(steps),
        timeout_secs: Some(600),
        ..Default::default()
    });
    let body = wf.to_registration_body(Some("proj-1")).unwrap();
    assert_eq!(body["name"], "My Workflow");
    assert_eq!(body["steps"].as_array().unwrap().len(), 2);
}

#[test]
fn test_workflow_define_with_invalid_steps_fails() {
    let steps = vec![
        job_step(
            "a",
            "j1",
            BaseStepOptions {
                depends_on: vec!["b".to_string()],
                ..Default::default()
            },
        ),
        job_step(
            "b",
            "j2",
            BaseStepOptions {
                depends_on: vec!["a".to_string()],
                ..Default::default()
            },
        ),
    ];
    let wf = define_workflow(WorkflowOptions {
        name: Some("Bad WF".to_string()),
        steps: Some(steps),
        ..Default::default()
    });
    let result = wf.to_registration_body(None);
    assert!(result.is_err());
}

// ---- Step API output integration tests ----

#[test]
fn test_step_to_api_comprehensive() {
    let step = job_step(
        "process",
        "job-process",
        BaseStepOptions {
            depends_on: vec!["fetch".to_string()],
            condition: Some("steps.fetch.output.status == 'ok'".to_string()),
            on_failure: Some("skip_dependents".to_string()),
            retry_max_attempts: Some(3),
            retry_backoff: Some("exponential".to_string()),
            resource_class: Some("large".to_string()),
            timeout_secs_override: Some(120),
            ..Default::default()
        },
    );
    let api = step.to_api();
    assert_eq!(api["ref"], "process");
    assert_eq!(api["type"], "job");
    assert_eq!(api["job_id"], "job-process");
    assert_eq!(api["depends_on"][0], "fetch");
    assert_eq!(api["condition"], "steps.fetch.output.status == 'ok'");
    assert_eq!(api["on_failure"], "skip_dependents");
    assert_eq!(api["retry_max_attempts"], 3);
    assert_eq!(api["resource_class"], "large");
}

// ---- Composition integration tests ----

#[test]
fn test_idempotency_workflow() {
    let mut headers = HashMap::new();
    with_idempotency(&mut headers, "idem-key-1");
    assert_eq!(headers.get("Idempotency-Key").unwrap(), "idem-key-1");

    let mut custom_headers = HashMap::new();
    with_idempotency_header(&mut custom_headers, "idem-key-2", "X-Idempotency");
    assert_eq!(custom_headers.get("X-Idempotency").unwrap(), "idem-key-2");
}

#[test]
fn test_retry_options_custom() {
    let opts = RetryOptions {
        attempts: 5,
        delay_ms: 500,
        factor: 1.5,
        max_delay_ms: 60_000,
        jitter: JitterStrategy::None,
    };
    assert_eq!(opts.attempts, 5);
    assert_eq!(opts.delay_ms, 500);
    assert_eq!(opts.jitter, JitterStrategy::None);
}

#[test]
fn test_strait_result_comprehensive() {
    let ok_result: StraitResult<String> = StraitResult::Ok("success".to_string());
    assert!(ok_result.is_ok());
    let value = ok_result.unwrap();
    assert_eq!(value, "success");

    let err_result = from_fn(|| -> Result<i32, std::io::Error> {
        Err(std::io::Error::new(
            std::io::ErrorKind::PermissionDenied,
            "denied",
        ))
    });
    assert!(err_result.is_err());
    let err = err_result.unwrap_err();
    assert_eq!(format!("{}", err), "denied");
}

// ---- Client builder integration tests ----

#[test]
fn test_client_builder_full_configuration() {
    let client = StraitClient::builder()
        .base_url("https://api.strait.dev/")
        .bearer_token("sk-test-token")
        .timeout_ms(15_000)
        .default_header("X-Request-Source", "sdk-test")
        .build();
    assert!(client.is_ok());
}

#[test]
fn test_client_builder_validation_no_url() {
    let result = StraitClient::builder().bearer_token("tok").build();
    assert!(result.is_err());
}

#[test]
fn test_client_builder_validation_no_auth() {
    let result = StraitClient::builder()
        .base_url("https://api.example.com")
        .build();
    assert!(result.is_err());
}

// ---- Cross-module integration tests ----

#[test]
fn test_all_terminal_statuses_prevent_further_transitions() {
    let terminal_statuses = vec![
        RunStatus::Completed,
        RunStatus::Canceled,
        RunStatus::Expired,
    ];
    let events = vec![
        RunEvent::Enqueue,
        RunEvent::Dequeue,
        RunEvent::Execute,
        RunEvent::Complete,
        RunEvent::Wait,
    ];
    for status in &terminal_statuses {
        for event in &events {
            assert!(
                !can_transition_run(*status, *event),
                "Terminal status {:?} should not transition on {:?}",
                status,
                event
            );
        }
    }
}

#[test]
fn test_workflow_terminal_statuses_exhaustive() {
    let all = vec![
        WorkflowRunStatus::Pending,
        WorkflowRunStatus::Running,
        WorkflowRunStatus::Paused,
        WorkflowRunStatus::Completed,
        WorkflowRunStatus::Failed,
        WorkflowRunStatus::TimedOut,
        WorkflowRunStatus::Canceled,
    ];
    let terminal_count = all
        .iter()
        .filter(|s| is_terminal_workflow_run_status(**s))
        .count();
    assert_eq!(terminal_count, 4);
}

#[test]
fn test_step_terminal_statuses_exhaustive() {
    let all = vec![
        StepRunStatus::Pending,
        StepRunStatus::Waiting,
        StepRunStatus::Running,
        StepRunStatus::Completed,
        StepRunStatus::Failed,
        StepRunStatus::Skipped,
        StepRunStatus::Canceled,
    ];
    let terminal_count = all
        .iter()
        .filter(|s| is_terminal_step_run_status(**s))
        .count();
    assert_eq!(terminal_count, 4);
}
