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
