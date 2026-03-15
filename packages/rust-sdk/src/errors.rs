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
