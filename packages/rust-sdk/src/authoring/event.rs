use serde_json::Value;

/// Type alias for event validator functions.
pub type EventValidator = Box<dyn Fn(Value) -> Result<Value, String> + Send + Sync>;

/// An event definition with key and optional validator.
pub struct EventDefinition {
    pub key: String,
    pub validate: Option<EventValidator>,
}

impl EventDefinition {
    pub fn parse(&self, input: Value) -> Result<Value, String> {
        if let Some(ref validate) = self.validate {
            validate(input)
        } else {
            Ok(input)
        }
    }
}

/// Create a new event definition.
pub fn define_event(
    key: impl Into<String>,
    validate: Option<EventValidator>,
) -> EventDefinition {
    EventDefinition {
        key: key.into(),
        validate,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_define_event_no_validator() {
        let event = define_event("user.created", None);
        assert_eq!(event.key, "user.created");
        let result = event.parse(json!({"name": "Alice"}));
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), json!({"name": "Alice"}));
    }

    #[test]
    fn test_define_event_with_validator() {
        let event = define_event(
            "order.placed",
            Some(Box::new(|v| {
                if v.get("amount").is_some() {
                    Ok(v)
                } else {
                    Err("missing amount field".to_string())
                }
            })),
        );
        let ok_result = event.parse(json!({"amount": 100}));
        assert!(ok_result.is_ok());
        let err_result = event.parse(json!({"name": "test"}));
        assert!(err_result.is_err());
    }

    #[test]
    fn test_event_key() {
        let event = define_event("payment.completed", None);
        assert_eq!(event.key, "payment.completed");
    }

    #[test]
    fn test_define_event_validator_passes_through() {
        let event = define_event(
            "transform",
            Some(Box::new(|v| {
                if let Some(n) = v.get("n").and_then(|n| n.as_i64()) {
                    Ok(json!({"n": n * 2}))
                } else {
                    Err("missing n".to_string())
                }
            })),
        );
        let result = event.parse(json!({"n": 5}));
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), json!({"n": 10}));
    }

    #[test]
    fn test_define_event_validator_error_message() {
        let event = define_event(
            "strict",
            Some(Box::new(|_| Err("always fails".to_string()))),
        );
        let result = event.parse(json!({}));
        assert_eq!(result.unwrap_err(), "always fails");
    }
}
