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
