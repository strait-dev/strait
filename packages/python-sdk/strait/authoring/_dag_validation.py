"""DAG validation using Kahn's algorithm."""

from __future__ import annotations

from typing import TYPE_CHECKING

from strait._errors import DagValidationError

if TYPE_CHECKING:
    from strait.authoring._steps import Step


def validate_dag(steps: list[Step]) -> list[str]:
    """Validate a DAG of workflow steps using Kahn's algorithm.

    Returns the topologically sorted step refs, or raises DagValidationError.
    """
    if not steps:
        return []

    refs = [s.step_ref() for s in steps]

    _check_duplicate_refs(refs)

    ref_set = set(refs)
    _check_missing_refs(steps, ref_set)

    return _topological_sort(steps, refs)


def _check_duplicate_refs(refs: list[str]) -> None:
    seen: set[str] = set()
    duplicates: list[str] = []
    for ref in refs:
        if ref in seen:
            duplicates.append(ref)
        seen.add(ref)

    if duplicates:
        raise DagValidationError(
            f"Duplicate step refs: {', '.join(duplicates)}",
            duplicate_refs=duplicates,
        )


def _check_missing_refs(steps: list[Step], all_refs: set[str]) -> None:
    missing: list[str] = []
    for step in steps:
        for dep in step.base_options().depends_on:
            if dep not in all_refs:
                missing.append(dep)

    if missing:
        raise DagValidationError(
            f"References to non-existent steps: {', '.join(missing)}",
            missing_refs=missing,
        )


def _topological_sort(steps: list[Step], refs: list[str]) -> list[str]:
    in_degree: dict[str, int] = {ref: 0 for ref in refs}
    adjacency: dict[str, list[str]] = {ref: [] for ref in refs}

    for step in steps:
        for dep in step.base_options().depends_on:
            adjacency[dep].append(step.step_ref())
            in_degree[step.step_ref()] += 1

    queue = [ref for ref in refs if in_degree[ref] == 0]
    sorted_refs: list[str] = []

    while queue:
        node = queue.pop(0)
        sorted_refs.append(node)
        for neighbor in adjacency[node]:
            in_degree[neighbor] -= 1
            if in_degree[neighbor] == 0:
                queue.append(neighbor)

    if len(sorted_refs) != len(steps):
        sorted_set = set(sorted_refs)
        in_cycle = [ref for ref in refs if ref not in sorted_set]
        raise DagValidationError(
            f"Circular dependency detected involving steps: {', '.join(in_cycle)}",
            cycles=in_cycle,
        )

    return sorted_refs
