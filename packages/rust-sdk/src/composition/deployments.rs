use crate::errors::StraitError;
use serde_json::Value;
use std::future::Future;

#[derive(Debug, Clone)]
pub struct CreateDeploymentVersionBody {
    pub project_id: Option<String>,
    pub environment: Option<String>,
    pub runtime: Option<String>,
    pub artifact_uri: Option<String>,
    pub manifest: Option<Value>,
    pub checksum: Option<String>,
}

#[derive(Debug, Clone)]
pub struct DeploymentVersionMutationBody {
    pub project_id: Option<String>,
    pub environment: Option<String>,
}

#[derive(Debug)]
pub struct CreateAndFinalizeOutput {
    pub created: Value,
    pub finalized: Value,
}

#[derive(Debug)]
pub struct CreateFinalizePromoteOutput {
    pub created: Value,
    pub finalized: Value,
    pub promoted: Value,
}

fn infer_mutation_body(create: &CreateDeploymentVersionBody) -> DeploymentVersionMutationBody {
    DeploymentVersionMutationBody {
        project_id: create.project_id.clone(),
        environment: create.environment.clone(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_deployment_version_body_default() {
        let body = CreateDeploymentVersionBody {
            project_id: None,
            environment: None,
            runtime: None,
            artifact_uri: None,
            manifest: None,
            checksum: None,
        };
        assert!(body.project_id.is_none());
        assert!(body.environment.is_none());
    }

    #[test]
    fn test_create_deployment_version_body_filled() {
        let body = CreateDeploymentVersionBody {
            project_id: Some("proj-1".to_string()),
            environment: Some("production".to_string()),
            runtime: Some("node18".to_string()),
            artifact_uri: Some("s3://bucket/artifact.tar.gz".to_string()),
            manifest: Some(serde_json::json!({"version": "1.0"})),
            checksum: Some("sha256:abc".to_string()),
        };
        assert_eq!(body.project_id.unwrap(), "proj-1");
        assert_eq!(body.environment.unwrap(), "production");
        assert_eq!(body.runtime.unwrap(), "node18");
    }

    #[test]
    fn test_deployment_version_mutation_body() {
        let body = DeploymentVersionMutationBody {
            project_id: Some("proj-1".to_string()),
            environment: Some("staging".to_string()),
        };
        assert_eq!(body.project_id.unwrap(), "proj-1");
        assert_eq!(body.environment.unwrap(), "staging");
    }

    #[test]
    fn test_infer_mutation_body() {
        let create = CreateDeploymentVersionBody {
            project_id: Some("proj-1".to_string()),
            environment: Some("production".to_string()),
            runtime: Some("node18".to_string()),
            artifact_uri: None,
            manifest: None,
            checksum: None,
        };
        let mutation = infer_mutation_body(&create);
        assert_eq!(mutation.project_id, Some("proj-1".to_string()));
        assert_eq!(mutation.environment, Some("production".to_string()));
    }

    #[test]
    fn test_infer_mutation_body_none_fields() {
        let create = CreateDeploymentVersionBody {
            project_id: None,
            environment: None,
            runtime: None,
            artifact_uri: None,
            manifest: None,
            checksum: None,
        };
        let mutation = infer_mutation_body(&create);
        assert!(mutation.project_id.is_none());
        assert!(mutation.environment.is_none());
    }
}
