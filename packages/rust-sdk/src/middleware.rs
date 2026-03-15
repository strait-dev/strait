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
