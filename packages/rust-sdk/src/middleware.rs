use std::collections::HashMap;

pub struct RequestContext {
    pub method: String,
    pub url: String,
    pub headers: HashMap<String, String>,
}

pub struct ResponseContext {
    pub method: String,
    pub url: String,
    pub status: u16,
    pub duration_ms: u64,
}

pub struct ErrorContext {
    pub method: String,
    pub url: String,
    pub error: String,
}

pub struct Middleware {
    pub on_request: Option<Box<dyn Fn(&RequestContext) + Send + Sync>>,
    pub on_response: Option<Box<dyn Fn(&ResponseContext) + Send + Sync>>,
    pub on_error: Option<Box<dyn Fn(&ErrorContext) + Send + Sync>>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_request_context_creation() {
        let ctx = RequestContext {
            method: "GET".to_string(),
            url: "https://api.example.com/v1/jobs".to_string(),
            headers: HashMap::new(),
        };
        assert_eq!(ctx.method, "GET");
        assert_eq!(ctx.url, "https://api.example.com/v1/jobs");
        assert!(ctx.headers.is_empty());
    }

    #[test]
    fn test_request_context_with_headers() {
        let mut headers = HashMap::new();
        headers.insert("Authorization".to_string(), "Bearer tok".to_string());
        let ctx = RequestContext {
            method: "POST".to_string(),
            url: "https://api.example.com/v1/jobs".to_string(),
            headers,
        };
        assert_eq!(ctx.headers.get("Authorization").unwrap(), "Bearer tok");
    }

    #[test]
    fn test_response_context_creation() {
        let ctx = ResponseContext {
            method: "GET".to_string(),
            url: "https://api.example.com/v1/jobs".to_string(),
            status: 200,
            duration_ms: 150,
        };
        assert_eq!(ctx.status, 200);
        assert_eq!(ctx.duration_ms, 150);
    }

    #[test]
    fn test_error_context_creation() {
        let ctx = ErrorContext {
            method: "POST".to_string(),
            url: "https://api.example.com/v1/jobs".to_string(),
            error: "connection refused".to_string(),
        };
        assert_eq!(ctx.error, "connection refused");
    }

    #[test]
    fn test_middleware_with_on_request() {
        let mw = Middleware {
            on_request: Some(Box::new(|_ctx| {})),
            on_response: None,
            on_error: None,
        };
        assert!(mw.on_request.is_some());
        assert!(mw.on_response.is_none());
        assert!(mw.on_error.is_none());
    }

    #[test]
    fn test_middleware_all_none() {
        let mw = Middleware {
            on_request: None,
            on_response: None,
            on_error: None,
        };
        assert!(mw.on_request.is_none());
        assert!(mw.on_response.is_none());
        assert!(mw.on_error.is_none());
    }

    #[test]
    fn test_middleware_on_request_invoked() {
        use std::sync::atomic::{AtomicBool, Ordering};
        use std::sync::Arc;
        let called = Arc::new(AtomicBool::new(false));
        let called_clone = called.clone();
        let mw = Middleware {
            on_request: Some(Box::new(move |_ctx| { called_clone.store(true, Ordering::SeqCst); })),
            on_response: None,
            on_error: None,
        };
        let ctx = RequestContext {
            method: "GET".to_string(),
            url: "https://test.com".to_string(),
            headers: HashMap::new(),
        };
        (mw.on_request.unwrap())(&ctx);
        assert!(called.load(Ordering::SeqCst));
    }

    #[test]
    fn test_middleware_on_response_invoked() {
        use std::sync::atomic::{AtomicU16, Ordering};
        use std::sync::Arc;
        let status_seen = Arc::new(AtomicU16::new(0));
        let s = status_seen.clone();
        let mw = Middleware {
            on_request: None,
            on_response: Some(Box::new(move |ctx| { s.store(ctx.status, Ordering::SeqCst); })),
            on_error: None,
        };
        let ctx = ResponseContext {
            method: "GET".to_string(),
            url: "https://test.com".to_string(),
            status: 201,
            duration_ms: 50,
        };
        (mw.on_response.unwrap())(&ctx);
        assert_eq!(status_seen.load(Ordering::SeqCst), 201);
    }
}
