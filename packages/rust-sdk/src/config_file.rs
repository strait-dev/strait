use std::env;
use std::fs;
use std::path::PathBuf;

use serde::Deserialize;

use crate::config::{normalize_base_url, AuthMode, AuthType, Config};
use crate::errors::StraitError;

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConfigFileSchema {
    #[serde(rename = "$schema")]
    #[allow(dead_code)]
    pub schema: Option<String>,
    pub project: Option<ConfigFileProject>,
    pub sdk: Option<ConfigFileSDK>,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConfigFileProject {
    pub id: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConfigFileSDK {
    pub base_url: Option<String>,
    pub auth_type: Option<String>,
    pub timeout_ms: Option<u64>,
}

fn find_config_file(path: Option<&str>, search_dir: Option<&str>) -> Option<PathBuf> {
    if let Some(p) = path {
        let pb = PathBuf::from(p);
        if pb.exists() {
            return Some(pb);
        }
        return None;
    }

    let start_dir = search_dir
        .map(PathBuf::from)
        .unwrap_or_else(|| env::current_dir().unwrap_or_else(|_| PathBuf::from(".")));

    let mut current = start_dir.as_path();
    loop {
        let candidate = current.join("strait.json");
        if candidate.exists() {
            return Some(candidate);
        }
        match current.parent() {
            Some(parent) => current = parent,
            None => return None,
        }
    }
}

fn read_config_file(path: Option<&str>, search_dir: Option<&str>) -> Result<Option<ConfigFileSchema>, StraitError> {
    let file_path = match find_config_file(path, search_dir) {
        Some(p) => p,
        None => return Ok(None),
    };

    let contents = fs::read_to_string(&file_path).map_err(|e| StraitError::Transport {
        message: format!("failed to read config file: {}", file_path.display()),
        cause: Some(e.to_string()),
    })?;

    let schema: ConfigFileSchema = serde_json::from_str(&contents).map_err(|e| StraitError::Decode {
        message: format!("failed to parse config file: {}", file_path.display()),
        body: Some(e.to_string()),
    })?;

    Ok(Some(schema))
}

pub fn config_from_file(path: Option<&str>, search_dir: Option<&str>) -> Result<Config, StraitError> {
    let schema = read_config_file(path, search_dir)?;

    let env_base_url = env::var("STRAIT_BASE_URL").ok();
    let env_api_key = env::var("STRAIT_API_KEY").ok();
    let env_auth_type = env::var("STRAIT_AUTH_TYPE").ok();
    let env_timeout_ms = env::var("STRAIT_TIMEOUT_MS").ok();

    let sdk = schema.as_ref().and_then(|s| s.sdk.as_ref());

    let base_url = env_base_url
        .or_else(|| sdk.and_then(|s| s.base_url.clone()))
        .ok_or_else(|| StraitError::Validation {
            message: "base_url is required (set STRAIT_BASE_URL or configure in strait.json)".to_string(),
            issues: vec!["base_url not found in environment or config file".to_string()],
        })?;

    let api_key = env_api_key.ok_or_else(|| StraitError::Validation {
        message: "STRAIT_API_KEY environment variable is required".to_string(),
        issues: vec!["STRAIT_API_KEY is required".to_string()],
    })?;

    let auth_type_str = env_auth_type
        .or_else(|| sdk.and_then(|s| s.auth_type.clone()))
        .unwrap_or_else(|| "bearer".to_string());
    let auth_type: AuthType = auth_type_str.parse()?;

    let timeout_ms = env_timeout_ms
        .and_then(|v| v.parse::<u64>().ok())
        .or_else(|| sdk.and_then(|s| s.timeout_ms))
        .unwrap_or(30000);

    Ok(Config {
        base_url: normalize_base_url(&base_url),
        auth: AuthMode {
            auth_type,
            token: api_key,
        },
        default_headers: std::collections::HashMap::new(),
        timeout_ms,
    })
}

pub fn project_id_from_file(path: Option<&str>, search_dir: Option<&str>) -> Result<Option<String>, StraitError> {
    let schema = read_config_file(path, search_dir)?;
    Ok(schema.and_then(|s| s.project).and_then(|p| p.id))
}
