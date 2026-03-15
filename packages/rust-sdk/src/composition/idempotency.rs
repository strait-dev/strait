use std::collections::HashMap;

pub fn with_idempotency(headers: &mut HashMap<String, String>, key: &str) {
    headers.insert("Idempotency-Key".to_string(), key.to_string());
}

pub fn with_idempotency_header(headers: &mut HashMap<String, String>, key: &str, header_name: &str) {
    headers.insert(header_name.to_string(), key.to_string());
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_with_idempotency() {
        let mut headers = HashMap::new();
        with_idempotency(&mut headers, "key-123");
        assert_eq!(headers.get("Idempotency-Key").unwrap(), "key-123");
    }

    #[test]
    fn test_with_idempotency_header_custom() {
        let mut headers = HashMap::new();
        with_idempotency_header(&mut headers, "key-123", "X-Custom-Key");
        assert_eq!(headers.get("X-Custom-Key").unwrap(), "key-123");
    }

    #[test]
    fn test_with_idempotency_preserves_existing() {
        let mut headers = HashMap::new();
        headers.insert("Existing".to_string(), "value".to_string());
        with_idempotency(&mut headers, "key-123");
        assert_eq!(headers.len(), 2);
        assert_eq!(headers.get("Existing").unwrap(), "value");
        assert_eq!(headers.get("Idempotency-Key").unwrap(), "key-123");
    }

    #[test]
    fn test_with_idempotency_overwrites() {
        let mut headers = HashMap::new();
        with_idempotency(&mut headers, "key-1");
        with_idempotency(&mut headers, "key-2");
        assert_eq!(headers.get("Idempotency-Key").unwrap(), "key-2");
        assert_eq!(headers.len(), 1);
    }

    #[test]
    fn test_with_idempotency_empty_key() {
        let mut headers = HashMap::new();
        with_idempotency(&mut headers, "");
        assert_eq!(headers.get("Idempotency-Key").unwrap(), "");
    }

    #[test]
    fn test_with_idempotency_header_empty_header_name() {
        let mut headers = HashMap::new();
        with_idempotency_header(&mut headers, "key", "");
        assert_eq!(headers.get("").unwrap(), "key");
    }
}
