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
