use std::collections::{HashMap, HashSet, VecDeque};
use crate::errors::StraitError;
use super::steps::Step;

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
