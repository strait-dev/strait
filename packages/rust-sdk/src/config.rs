use std::collections::HashMap;
use std::env;

use serde::{Deserialize, Serialize};

use crate::errors::StraitError;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum AuthType {
    #[serde(rename = "bearer")]
    Bearer,
    #[serde(rename = "apiKey")]
    ApiKey,
    #[serde(rename = "runToken")]
    RunToken,
}

impl std::fmt::Display for AuthType {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AuthType::Bearer => write!(f, "bearer"),
            AuthType::ApiKey => write!(f, "apiKey"),
            AuthType::RunToken => write!(f, "runToken"),
        }
    }
}

impl std::str::FromStr for AuthType {
    type Err = StraitError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "bearer" => Ok(AuthType::Bearer),
            "apiKey" => Ok(AuthType::ApiKey),
            "runToken" => Ok(AuthType::RunToken),
            _ => Err(StraitError::Validation {
                message: format!("invalid auth type: {s}"),
                issues: vec![format!("expected one of: bearer, apiKey, runToken; got: {s}")],
            }),
        }
    }
}

#[derive(Debug, Clone)]
pub struct AuthMode {
    pub auth_type: AuthType,
    pub token: String,
}

#[derive(Debug, Clone)]
pub struct Config {
    pub base_url: String,
    pub auth: AuthMode,
    pub default_headers: HashMap<String, String>,
    pub timeout_ms: u64,
}

pub fn normalize_base_url(url: &str) -> String {
    url.trim_end_matches('/').to_string()
}

pub fn get_authorization_header(auth: &AuthMode) -> String {
    format!("Bearer {}", auth.token)
}

pub fn config_from_env() -> Result<Config, StraitError> {
    let base_url = env::var("STRAIT_BASE_URL").map_err(|_| StraitError::Validation {
        message: "STRAIT_BASE_URL environment variable is not set".to_string(),
        issues: vec!["STRAIT_BASE_URL is required".to_string()],
    })?;

    let api_key = env::var("STRAIT_API_KEY").map_err(|_| StraitError::Validation {
        message: "STRAIT_API_KEY environment variable is not set".to_string(),
        issues: vec!["STRAIT_API_KEY is required".to_string()],
    })?;

    let auth_type_str = env::var("STRAIT_AUTH_TYPE").unwrap_or_else(|_| "bearer".to_string());
    let auth_type: AuthType = auth_type_str.parse()?;

    let timeout_ms: u64 = env::var("STRAIT_TIMEOUT_MS")
        .unwrap_or_else(|_| "30000".to_string())
        .parse()
        .map_err(|_| StraitError::Validation {
            message: "STRAIT_TIMEOUT_MS must be a valid number".to_string(),
            issues: vec!["STRAIT_TIMEOUT_MS must be a u64".to_string()],
        })?;

    Ok(Config {
        base_url: normalize_base_url(&base_url),
        auth: AuthMode {
            auth_type,
            token: api_key,
        },
        default_headers: HashMap::new(),
        timeout_ms,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_auth_type_display_bearer() {
        assert_eq!(format!("{}", AuthType::Bearer), "bearer");
    }

    #[test]
    fn test_auth_type_display_api_key() {
        assert_eq!(format!("{}", AuthType::ApiKey), "apiKey");
    }

    #[test]
    fn test_auth_type_display_run_token() {
        assert_eq!(format!("{}", AuthType::RunToken), "runToken");
    }

    #[test]
    fn test_auth_type_from_str_bearer() {
        let at: AuthType = "bearer".parse().unwrap();
        assert_eq!(at, AuthType::Bearer);
    }

    #[test]
    fn test_auth_type_from_str_api_key() {
        let at: AuthType = "apiKey".parse().unwrap();
        assert_eq!(at, AuthType::ApiKey);
    }

    #[test]
    fn test_auth_type_from_str_run_token() {
        let at: AuthType = "runToken".parse().unwrap();
        assert_eq!(at, AuthType::RunToken);
    }

    #[test]
    fn test_auth_type_from_str_invalid() {
        let result: Result<AuthType, _> = "invalid".parse();
        assert!(result.is_err());
    }

    #[test]
    fn test_auth_type_from_str_error_message() {
        let result: Result<AuthType, _> = "bad".parse();
        match result.unwrap_err() {
            StraitError::Validation { message, issues } => {
                assert!(message.contains("invalid auth type"));
                assert!(!issues.is_empty());
            }
            _ => panic!("expected Validation error"),
        }
    }

    #[test]
    fn test_normalize_base_url_strips_trailing_slash() {
        assert_eq!(normalize_base_url("https://api.example.com/"), "https://api.example.com");
    }

    #[test]
    fn test_normalize_base_url_strips_multiple_slashes() {
        assert_eq!(normalize_base_url("https://api.example.com///"), "https://api.example.com");
    }

    #[test]
    fn test_normalize_base_url_no_trailing_slash() {
        assert_eq!(normalize_base_url("https://api.example.com"), "https://api.example.com");
    }

    #[test]
    fn test_normalize_base_url_empty() {
        assert_eq!(normalize_base_url(""), "");
    }

    #[test]
    fn test_normalize_base_url_single_slash() {
        assert_eq!(normalize_base_url("/"), "");
    }

    #[test]
    fn test_get_authorization_header_bearer() {
        let auth = AuthMode { auth_type: AuthType::Bearer, token: "tok123".to_string() };
        assert_eq!(get_authorization_header(&auth), "Bearer tok123");
    }

    #[test]
    fn test_get_authorization_header_api_key() {
        let auth = AuthMode { auth_type: AuthType::ApiKey, token: "key-abc".to_string() };
        assert_eq!(get_authorization_header(&auth), "Bearer key-abc");
    }

    #[test]
    fn test_config_default_timeout() {
        let config = Config {
            base_url: "https://api.example.com".to_string(),
            auth: AuthMode { auth_type: AuthType::ApiKey, token: "key".to_string() },
            default_headers: HashMap::new(),
            timeout_ms: 30_000,
        };
        assert_eq!(config.timeout_ms, 30_000);
    }

    #[test]
    fn test_auth_mode_creation() {
        let auth = AuthMode { auth_type: AuthType::ApiKey, token: "my-key".to_string() };
        assert_eq!(auth.token, "my-key");
        assert_eq!(format!("{}", auth.auth_type), "apiKey");
    }

    #[test]
    fn test_config_base_url_stored() {
        let config = Config {
            base_url: "https://test.io".to_string(),
            auth: AuthMode { auth_type: AuthType::Bearer, token: "t".to_string() },
            default_headers: HashMap::new(),
            timeout_ms: 5000,
        };
        assert_eq!(config.base_url, "https://test.io");
    }

    #[test]
    fn test_config_default_headers_empty() {
        let config = Config {
            base_url: "https://test.io".to_string(),
            auth: AuthMode { auth_type: AuthType::Bearer, token: "t".to_string() },
            default_headers: HashMap::new(),
            timeout_ms: 5000,
        };
        assert!(config.default_headers.is_empty());
    }

    #[test]
    fn test_config_default_headers_with_entries() {
        let mut headers = HashMap::new();
        headers.insert("X-Custom".to_string(), "value".to_string());
        let config = Config {
            base_url: "https://test.io".to_string(),
            auth: AuthMode { auth_type: AuthType::Bearer, token: "t".to_string() },
            default_headers: headers,
            timeout_ms: 5000,
        };
        assert_eq!(config.default_headers.get("X-Custom").unwrap(), "value");
    }

    #[test]
    fn test_auth_type_serde_roundtrip() {
        let json = serde_json::to_string(&AuthType::Bearer).unwrap();
        let deserialized: AuthType = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized, AuthType::Bearer);
    }

    #[test]
    fn test_auth_type_serde_api_key() {
        let json = serde_json::to_string(&AuthType::ApiKey).unwrap();
        assert_eq!(json, "\"apiKey\"");
    }

    #[test]
    fn test_auth_type_serde_run_token() {
        let json = serde_json::to_string(&AuthType::RunToken).unwrap();
        assert_eq!(json, "\"runToken\"");
    }
}
