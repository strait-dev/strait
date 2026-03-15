use std::collections::HashMap;

pub fn with_idempotency(headers: &mut HashMap<String, String>, key: &str) {
    headers.insert("Idempotency-Key".to_string(), key.to_string());
}

pub fn with_idempotency_header(headers: &mut HashMap<String, String>, key: &str, header_name: &str) {
    headers.insert(header_name.to_string(), key.to_string());
}
