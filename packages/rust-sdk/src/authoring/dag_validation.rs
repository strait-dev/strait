use super::steps::Step;
use crate::errors::StraitError;
use std::collections::{HashMap, HashSet, VecDeque};

pub fn validate_dag(steps: &[Step]) -> Result<Vec<String>, StraitError> {
    let refs: Vec<String> = steps.iter().map(|s| s.step_ref().to_string()).collect();

    // Check duplicate refs
    let mut seen = HashSet::new();
    let mut duplicates = Vec::new();
    for r in &refs {
        if !seen.insert(r.as_str()) {
            duplicates.push(r.clone());
        }
    }
    if !duplicates.is_empty() {
        return Err(StraitError::DagValidation {
            message: format!("duplicate step refs: {}", duplicates.join(", ")),
            cycles: vec![],
            missing_refs: vec![],
            duplicate_refs: duplicates,
        });
    }

    let all_refs: HashSet<&str> = refs.iter().map(|s| s.as_str()).collect();

    // Check missing refs
    let mut missing = Vec::new();
    for step in steps {
        for dep in step.depends_on() {
            if !all_refs.contains(dep.as_str()) {
                missing.push(dep.clone());
            }
        }
    }
    if !missing.is_empty() {
        return Err(StraitError::DagValidation {
            message: format!("missing step refs: {}", missing.join(", ")),
            cycles: vec![],
            missing_refs: missing,
            duplicate_refs: vec![],
        });
    }

    // Topological sort (Kahn's algorithm)
    let mut in_degree: HashMap<&str, usize> = HashMap::new();
    for r in &refs {
        in_degree.insert(r.as_str(), 0);
    }

    let mut adj: HashMap<&str, Vec<&str>> = HashMap::new();
    for step in steps {
        for dep in step.depends_on() {
            *in_degree.entry(step.step_ref()).or_insert(0) += 1;
            adj.entry(dep.as_str()).or_default().push(step.step_ref());
        }
    }

    let mut queue: VecDeque<&str> = VecDeque::new();
    for r in &refs {
        if *in_degree.get(r.as_str()).unwrap_or(&0) == 0 {
            queue.push_back(r.as_str());
        }
    }

    let mut sorted = Vec::new();
    while let Some(node) = queue.pop_front() {
        sorted.push(node.to_string());
        if let Some(neighbors) = adj.get(node) {
            for neighbor in neighbors {
                if let Some(deg) = in_degree.get_mut(neighbor) {
                    *deg -= 1;
                    if *deg == 0 {
                        queue.push_back(neighbor);
                    }
                }
            }
        }
    }

    if sorted.len() != refs.len() {
        let sorted_set: HashSet<&str> = sorted.iter().map(|s| s.as_str()).collect();
        let cycles: Vec<String> = refs
            .iter()
            .filter(|r| !sorted_set.contains(r.as_str()))
            .cloned()
            .collect();
        return Err(StraitError::DagValidation {
            message: format!("DAG contains cycles involving: {}", cycles.join(", ")),
            cycles,
            missing_refs: vec![],
            duplicate_refs: vec![],
        });
    }

    Ok(sorted)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::authoring::steps::{BaseStepOptions, job_step};

    #[test]
    fn test_valid_linear_dag() {
        let steps = vec![
            job_step("s1", "j1", BaseStepOptions::default()),
            job_step(
                "s2",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["s1".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "s3",
                "j3",
                BaseStepOptions {
                    depends_on: vec!["s2".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let result = validate_dag(&steps);
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), vec!["s1", "s2", "s3"]);
    }

    #[test]
    fn test_valid_diamond_dag() {
        let steps = vec![
            job_step("s1", "j1", BaseStepOptions::default()),
            job_step(
                "s2",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["s1".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "s3",
                "j3",
                BaseStepOptions {
                    depends_on: vec!["s1".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "s4",
                "j4",
                BaseStepOptions {
                    depends_on: vec!["s2".to_string(), "s3".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let result = validate_dag(&steps);
        assert!(result.is_ok());
    }

    #[test]
    fn test_single_step() {
        let steps = vec![job_step("s1", "j1", BaseStepOptions::default())];
        assert!(validate_dag(&steps).is_ok());
    }

    #[test]
    fn test_empty_steps() {
        let steps: Vec<Step> = vec![];
        assert!(validate_dag(&steps).is_ok());
    }

    #[test]
    fn test_cycle_detection_two_nodes() {
        let steps = vec![
            job_step(
                "a",
                "j1",
                BaseStepOptions {
                    depends_on: vec!["b".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "b",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["a".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let result = validate_dag(&steps);
        assert!(result.is_err());
        match result.unwrap_err() {
            StraitError::DagValidation { cycles, .. } => {
                assert!(!cycles.is_empty());
            }
            _ => panic!("expected DagValidation error"),
        }
    }

    #[test]
    fn test_three_node_cycle() {
        let steps = vec![
            job_step(
                "a",
                "j1",
                BaseStepOptions {
                    depends_on: vec!["c".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "b",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["a".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "c",
                "j3",
                BaseStepOptions {
                    depends_on: vec!["b".to_string()],
                    ..Default::default()
                },
            ),
        ];
        assert!(validate_dag(&steps).is_err());
    }

    #[test]
    fn test_duplicate_refs() {
        let steps = vec![
            job_step("s1", "j1", BaseStepOptions::default()),
            job_step("s1", "j2", BaseStepOptions::default()),
        ];
        let result = validate_dag(&steps);
        assert!(result.is_err());
        match result.unwrap_err() {
            StraitError::DagValidation { duplicate_refs, .. } => {
                assert!(duplicate_refs.contains(&"s1".to_string()));
            }
            _ => panic!("expected DagValidation error"),
        }
    }

    #[test]
    fn test_missing_refs() {
        let steps = vec![job_step(
            "s1",
            "j1",
            BaseStepOptions {
                depends_on: vec!["nonexistent".to_string()],
                ..Default::default()
            },
        )];
        let result = validate_dag(&steps);
        assert!(result.is_err());
        match result.unwrap_err() {
            StraitError::DagValidation { missing_refs, .. } => {
                assert!(missing_refs.contains(&"nonexistent".to_string()));
            }
            _ => panic!("expected DagValidation error"),
        }
    }

    #[test]
    fn test_parallel_steps_no_deps() {
        let steps = vec![
            job_step("s1", "j1", BaseStepOptions::default()),
            job_step("s2", "j2", BaseStepOptions::default()),
            job_step("s3", "j3", BaseStepOptions::default()),
        ];
        assert!(validate_dag(&steps).is_ok());
    }

    #[test]
    fn test_self_dependency() {
        let steps = vec![job_step(
            "s1",
            "j1",
            BaseStepOptions {
                depends_on: vec!["s1".to_string()],
                ..Default::default()
            },
        )];
        assert!(validate_dag(&steps).is_err());
    }

    #[test]
    fn test_multiple_missing_refs() {
        let steps = vec![job_step(
            "s1",
            "j1",
            BaseStepOptions {
                depends_on: vec!["x".to_string(), "y".to_string()],
                ..Default::default()
            },
        )];
        let result = validate_dag(&steps);
        match result.unwrap_err() {
            StraitError::DagValidation { missing_refs, .. } => {
                assert_eq!(missing_refs.len(), 2);
            }
            _ => panic!("expected DagValidation error"),
        }
    }

    #[test]
    fn test_topological_order_preserved() {
        let steps = vec![
            job_step("a", "j1", BaseStepOptions::default()),
            job_step(
                "b",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["a".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let sorted = validate_dag(&steps).unwrap();
        let a_pos = sorted.iter().position(|s| s == "a").unwrap();
        let b_pos = sorted.iter().position(|s| s == "b").unwrap();
        assert!(a_pos < b_pos);
    }

    #[test]
    fn test_diamond_topological_order() {
        let steps = vec![
            job_step("root", "j1", BaseStepOptions::default()),
            job_step(
                "left",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["root".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "right",
                "j3",
                BaseStepOptions {
                    depends_on: vec!["root".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "sink",
                "j4",
                BaseStepOptions {
                    depends_on: vec!["left".to_string(), "right".to_string()],
                    ..Default::default()
                },
            ),
        ];
        let sorted = validate_dag(&steps).unwrap();
        let root_pos = sorted.iter().position(|s| s == "root").unwrap();
        let sink_pos = sorted.iter().position(|s| s == "sink").unwrap();
        assert!(root_pos < sink_pos);
    }

    #[test]
    fn test_cycle_error_message_contains_involved_nodes() {
        let steps = vec![
            job_step(
                "x",
                "j1",
                BaseStepOptions {
                    depends_on: vec!["y".to_string()],
                    ..Default::default()
                },
            ),
            job_step(
                "y",
                "j2",
                BaseStepOptions {
                    depends_on: vec!["x".to_string()],
                    ..Default::default()
                },
            ),
        ];
        match validate_dag(&steps).unwrap_err() {
            StraitError::DagValidation { message, .. } => {
                assert!(message.contains("cycles"));
            }
            _ => panic!("expected DagValidation"),
        }
    }

    #[test]
    fn test_duplicate_error_message() {
        let steps = vec![
            job_step("dup", "j1", BaseStepOptions::default()),
            job_step("dup", "j2", BaseStepOptions::default()),
        ];
        match validate_dag(&steps).unwrap_err() {
            StraitError::DagValidation { message, .. } => {
                assert!(message.contains("duplicate"));
            }
            _ => panic!("expected DagValidation"),
        }
    }
}
