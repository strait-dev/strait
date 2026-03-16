use serde_json::Value;

#[derive(thiserror::Error, Debug)]
pub enum StraitError {
    #[error("transport error: {message}")]
    Transport {
        message: String,
        cause: Option<String>,
    },

    #[error("decode error: {message}")]
    Decode {
        message: String,
        body: Option<String>,
    },

    #[error("validation error: {message}")]
    Validation {
        message: String,
        issues: Vec<String>,
    },

    #[error("unauthorized: {message}")]
    Unauthorized {
        status: u16,
        message: String,
        body: Option<Value>,
    },

    #[error("not found: {message}")]
    NotFound {
        status: u16,
        message: String,
        body: Option<Value>,
    },

    #[error("conflict: {message}")]
    Conflict {
        status: u16,
        message: String,
        body: Option<Value>,
    },

    #[error("rate limited: {message}")]
    RateLimited {
        status: u16,
        message: String,
        body: Option<Value>,
    },

    #[error("api error: {message}")]
    Api {
        status: u16,
        message: String,
        body: Option<Value>,
    },

    #[error("timeout: {message}")]
    Timeout {
        message: String,
        run_id: Option<String>,
        elapsed_ms: Option<u64>,
    },

    #[error("DAG validation error: {message}")]
    DagValidation {
        message: String,
        cycles: Vec<String>,
        missing_refs: Vec<String>,
        duplicate_refs: Vec<String>,
    },

    #[error("cost budget exceeded: {message}")]
    CostBudgetExceeded {
        message: String,
        current_cost_microusd: i64,
        max_cost_microusd: i64,
    },
}

pub fn map_http_error(status: u16, message: String, body: Option<Value>) -> StraitError {
    match status {
        401 | 403 => StraitError::Unauthorized {
            status,
            message,
            body,
        },
        404 => StraitError::NotFound {
            status,
            message,
            body,
        },
        409 => StraitError::Conflict {
            status,
            message,
            body,
        },
        429 => StraitError::RateLimited {
            status,
            message,
            body,
        },
        _ => StraitError::Api {
            status,
            message,
            body,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_map_http_error_401() {
        let err = map_http_error(401, "unauthorized".to_string(), None);
        assert!(matches!(err, StraitError::Unauthorized { status: 401, .. }));
    }

    #[test]
    fn test_map_http_error_403() {
        let err = map_http_error(403, "forbidden".to_string(), None);
        assert!(matches!(err, StraitError::Unauthorized { status: 403, .. }));
    }

    #[test]
    fn test_map_http_error_404() {
        let err = map_http_error(404, "not found".to_string(), None);
        assert!(matches!(err, StraitError::NotFound { status: 404, .. }));
    }

    #[test]
    fn test_map_http_error_409() {
        let err = map_http_error(409, "conflict".to_string(), None);
        assert!(matches!(err, StraitError::Conflict { status: 409, .. }));
    }

    #[test]
    fn test_map_http_error_429() {
        let err = map_http_error(429, "rate limited".to_string(), None);
        assert!(matches!(err, StraitError::RateLimited { status: 429, .. }));
    }

    #[test]
    fn test_map_http_error_500() {
        let err = map_http_error(500, "server error".to_string(), None);
        assert!(matches!(err, StraitError::Api { status: 500, .. }));
    }

    #[test]
    fn test_map_http_error_502() {
        let err = map_http_error(502, "bad gateway".to_string(), None);
        assert!(matches!(err, StraitError::Api { status: 502, .. }));
    }

    #[test]
    fn test_map_http_error_503() {
        let err = map_http_error(503, "unavailable".to_string(), None);
        assert!(matches!(err, StraitError::Api { status: 503, .. }));
    }

    #[test]
    fn test_map_http_error_with_body() {
        let body = Some(serde_json::json!({"error": "bad"}));
        let err = map_http_error(404, "not found".to_string(), body.clone());
        match err {
            StraitError::NotFound { body: b, .. } => assert_eq!(b, body),
            _ => panic!("expected NotFound"),
        }
    }

    #[test]
    fn test_map_http_error_401_with_body() {
        let body = Some(serde_json::json!({"reason": "token expired"}));
        let err = map_http_error(401, "unauth".to_string(), body.clone());
        match err {
            StraitError::Unauthorized {
                body: b, message, ..
            } => {
                assert_eq!(b, body);
                assert_eq!(message, "unauth");
            }
            _ => panic!("expected Unauthorized"),
        }
    }

    #[test]
    fn test_map_http_error_409_message() {
        let err = map_http_error(409, "already exists".to_string(), None);
        match err {
            StraitError::Conflict { message, .. } => assert_eq!(message, "already exists"),
            _ => panic!("expected Conflict"),
        }
    }

    #[test]
    fn test_map_http_error_429_message() {
        let err = map_http_error(429, "slow down".to_string(), None);
        match err {
            StraitError::RateLimited { message, .. } => assert_eq!(message, "slow down"),
            _ => panic!("expected RateLimited"),
        }
    }

    #[test]
    fn test_error_display_transport() {
        let err = StraitError::Transport {
            message: "conn refused".to_string(),
            cause: None,
        };
        assert_eq!(format!("{}", err), "transport error: conn refused");
    }

    #[test]
    fn test_error_display_decode() {
        let err = StraitError::Decode {
            message: "invalid json".to_string(),
            body: None,
        };
        assert_eq!(format!("{}", err), "decode error: invalid json");
    }

    #[test]
    fn test_error_display_validation() {
        let err = StraitError::Validation {
            message: "bad input".to_string(),
            issues: vec!["field required".to_string()],
        };
        assert_eq!(format!("{}", err), "validation error: bad input");
    }

    #[test]
    fn test_error_display_unauthorized() {
        let err = StraitError::Unauthorized {
            status: 401,
            message: "no token".to_string(),
            body: None,
        };
        assert_eq!(format!("{}", err), "unauthorized: no token");
    }

    #[test]
    fn test_error_display_not_found() {
        let err = StraitError::NotFound {
            status: 404,
            message: "missing".to_string(),
            body: None,
        };
        assert_eq!(format!("{}", err), "not found: missing");
    }

    #[test]
    fn test_error_display_conflict() {
        let err = StraitError::Conflict {
            status: 409,
            message: "dup".to_string(),
            body: None,
        };
        assert_eq!(format!("{}", err), "conflict: dup");
    }

    #[test]
    fn test_error_display_rate_limited() {
        let err = StraitError::RateLimited {
            status: 429,
            message: "slow".to_string(),
            body: None,
        };
        assert_eq!(format!("{}", err), "rate limited: slow");
    }

    #[test]
    fn test_error_display_api() {
        let err = StraitError::Api {
            status: 500,
            message: "oops".to_string(),
            body: None,
        };
        assert_eq!(format!("{}", err), "api error: oops");
    }

    #[test]
    fn test_error_display_timeout() {
        let err = StraitError::Timeout {
            message: "timed out".to_string(),
            run_id: Some("r1".to_string()),
            elapsed_ms: Some(5000),
        };
        assert_eq!(format!("{}", err), "timeout: timed out");
    }

    #[test]
    fn test_error_display_dag() {
        let err = StraitError::DagValidation {
            message: "cycles".to_string(),
            cycles: vec!["a".to_string()],
            missing_refs: vec![],
            duplicate_refs: vec![],
        };
        assert_eq!(format!("{}", err), "DAG validation error: cycles");
    }

    #[test]
    fn test_transport_error_cause() {
        let err = StraitError::Transport {
            message: "fail".to_string(),
            cause: Some("dns".to_string()),
        };
        match err {
            StraitError::Transport { cause, .. } => assert_eq!(cause, Some("dns".to_string())),
            _ => panic!("expected Transport"),
        }
    }

    #[test]
    fn test_decode_error_body() {
        let err = StraitError::Decode {
            message: "parse".to_string(),
            body: Some("raw text".to_string()),
        };
        match err {
            StraitError::Decode { body, .. } => assert_eq!(body, Some("raw text".to_string())),
            _ => panic!("expected Decode"),
        }
    }

    #[test]
    fn test_validation_issues() {
        let err = StraitError::Validation {
            message: "bad".to_string(),
            issues: vec!["a".to_string(), "b".to_string()],
        };
        match err {
            StraitError::Validation { issues, .. } => assert_eq!(issues.len(), 2),
            _ => panic!("expected Validation"),
        }
    }

    #[test]
    fn test_timeout_fields() {
        let err = StraitError::Timeout {
            message: "slow".to_string(),
            run_id: Some("run-1".to_string()),
            elapsed_ms: Some(10000),
        };
        match err {
            StraitError::Timeout {
                run_id, elapsed_ms, ..
            } => {
                assert_eq!(run_id, Some("run-1".to_string()));
                assert_eq!(elapsed_ms, Some(10000));
            }
            _ => panic!("expected Timeout"),
        }
    }

    #[test]
    fn test_error_display_cost_budget_exceeded() {
        let err = StraitError::CostBudgetExceeded {
            message: "over budget".to_string(),
            current_cost_microusd: 150_000,
            max_cost_microusd: 100_000,
        };
        assert_eq!(format!("{}", err), "cost budget exceeded: over budget");
    }

    #[test]
    fn test_cost_budget_exceeded_fields() {
        let err = StraitError::CostBudgetExceeded {
            message: "exceeded".to_string(),
            current_cost_microusd: 200_000,
            max_cost_microusd: 100_000,
        };
        match err {
            StraitError::CostBudgetExceeded {
                current_cost_microusd,
                max_cost_microusd,
                ..
            } => {
                assert_eq!(current_cost_microusd, 200_000);
                assert_eq!(max_cost_microusd, 100_000);
            }
            _ => panic!("expected CostBudgetExceeded"),
        }
    }

    #[test]
    fn test_dag_validation_all_fields() {
        let err = StraitError::DagValidation {
            message: "bad dag".to_string(),
            cycles: vec!["c1".to_string()],
            missing_refs: vec!["m1".to_string()],
            duplicate_refs: vec!["d1".to_string()],
        };
        match err {
            StraitError::DagValidation {
                cycles,
                missing_refs,
                duplicate_refs,
                ..
            } => {
                assert_eq!(cycles, vec!["c1"]);
                assert_eq!(missing_refs, vec!["m1"]);
                assert_eq!(duplicate_refs, vec!["d1"]);
            }
            _ => panic!("expected DagValidation"),
        }
    }
}
