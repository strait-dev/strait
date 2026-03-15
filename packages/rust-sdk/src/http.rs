use std::collections::HashMap;

pub fn substitute_path_params(path: &str, params: &[(&str, &str)]) -> String {
    let mut result = path.to_string();
    for (key, value) in params {
        result = result.replace(&format!("{{{}}}", key), value);
    }
    result
}

#[derive(Debug, Clone)]
pub struct RequestOptions {
    pub method: String,
    pub path: String,
    pub query: Option<Vec<(String, String)>>,
    pub headers: Option<HashMap<String, String>>,
    pub body: Option<serde_json::Value>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_substitute_single_param() {
        let result = substitute_path_params("/v1/jobs/{jobID}", &[("jobID", "j123")]);
        assert_eq!(result, "/v1/jobs/j123");
    }

    #[test]
    fn test_substitute_multiple_params() {
        let result = substitute_path_params(
            "/v1/jobs/{jobID}/versions/{versionID}",
            &[("jobID", "j1"), ("versionID", "v2")],
        );
        assert_eq!(result, "/v1/jobs/j1/versions/v2");
    }

    #[test]
    fn test_substitute_no_params() {
        let result = substitute_path_params("/v1/health", &[]);
        assert_eq!(result, "/v1/health");
    }

    #[test]
    fn test_substitute_missing_param() {
        let result = substitute_path_params("/v1/jobs/{jobID}", &[("other", "val")]);
        assert_eq!(result, "/v1/jobs/{jobID}");
    }

    #[test]
    fn test_substitute_empty_value() {
        let result = substitute_path_params("/v1/jobs/{jobID}", &[("jobID", "")]);
        assert_eq!(result, "/v1/jobs/");
    }

    #[test]
    fn test_substitute_preserves_prefix() {
        let result = substitute_path_params("/v1/runs/{runID}/events", &[("runID", "r1")]);
        assert_eq!(result, "/v1/runs/r1/events");
    }

    #[test]
    fn test_substitute_special_chars_in_value() {
        let result = substitute_path_params("/v1/jobs/{jobID}", &[("jobID", "job-123_abc")]);
        assert_eq!(result, "/v1/jobs/job-123_abc");
    }

    #[test]
    fn test_substitute_repeated_param() {
        let result = substitute_path_params("/v1/{id}/sub/{id}", &[("id", "x")]);
        assert_eq!(result, "/v1/x/sub/x");
    }

    #[test]
    fn test_request_options_creation() {
        let opts = RequestOptions {
            method: "GET".to_string(),
            path: "/v1/jobs".to_string(),
            query: None,
            headers: None,
            body: None,
        };
        assert_eq!(opts.method, "GET");
        assert_eq!(opts.path, "/v1/jobs");
        assert!(opts.query.is_none());
        assert!(opts.headers.is_none());
        assert!(opts.body.is_none());
    }

    #[test]
    fn test_request_options_with_query() {
        let opts = RequestOptions {
            method: "GET".to_string(),
            path: "/v1/jobs".to_string(),
            query: Some(vec![("limit".to_string(), "10".to_string())]),
            headers: None,
            body: None,
        };
        assert_eq!(opts.query.unwrap().len(), 1);
    }

    #[test]
    fn test_request_options_with_body() {
        let opts = RequestOptions {
            method: "POST".to_string(),
            path: "/v1/jobs".to_string(),
            query: None,
            headers: None,
            body: Some(serde_json::json!({"name": "test"})),
        };
        assert_eq!(opts.body.unwrap()["name"], "test");
    }

    #[test]
    fn test_request_options_with_headers() {
        let mut h = HashMap::new();
        h.insert("X-Custom".to_string(), "val".to_string());
        let opts = RequestOptions {
            method: "GET".to_string(),
            path: "/v1/jobs".to_string(),
            query: None,
            headers: Some(h),
            body: None,
        };
        assert_eq!(opts.headers.unwrap().get("X-Custom").unwrap(), "val");
    }
}
