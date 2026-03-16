"""Tests for DAG validation."""

import pytest

from strait._errors import DagValidationError
from strait.authoring._dag_validation import validate_dag
from strait.authoring._steps import job_step


class TestValidateDag:
    def test_empty_dag(self):
        assert validate_dag([]) == []

    def test_single_step(self):
        result = validate_dag([job_step("a", "j1")])
        assert result == ["a"]

    def test_linear_chain(self):
        steps = [
            job_step("a", "j1"),
            job_step("b", "j2", depends_on=["a"]),
            job_step("c", "j3", depends_on=["b"]),
        ]
        result = validate_dag(steps)
        assert result.index("a") < result.index("b") < result.index("c")

    def test_diamond_dag(self):
        steps = [
            job_step("a", "j1"),
            job_step("b", "j2", depends_on=["a"]),
            job_step("c", "j3", depends_on=["a"]),
            job_step("d", "j4", depends_on=["b", "c"]),
        ]
        result = validate_dag(steps)
        assert result.index("a") < result.index("b")
        assert result.index("a") < result.index("c")
        assert result.index("b") < result.index("d")
        assert result.index("c") < result.index("d")

    def test_parallel_steps(self):
        steps = [
            job_step("a", "j1"),
            job_step("b", "j2"),
            job_step("c", "j3"),
        ]
        result = validate_dag(steps)
        assert set(result) == {"a", "b", "c"}

    def test_duplicate_refs_raises(self):
        steps = [
            job_step("a", "j1"),
            job_step("a", "j2"),
        ]
        with pytest.raises(DagValidationError) as exc_info:
            validate_dag(steps)
        assert "Duplicate" in str(exc_info.value)
        assert exc_info.value.duplicate_refs == ["a"]

    def test_missing_refs_raises(self):
        steps = [
            job_step("a", "j1", depends_on=["missing"]),
        ]
        with pytest.raises(DagValidationError) as exc_info:
            validate_dag(steps)
        assert "non-existent" in str(exc_info.value)
        assert exc_info.value.missing_refs == ["missing"]

    def test_cycle_raises(self):
        steps = [
            job_step("a", "j1", depends_on=["b"]),
            job_step("b", "j2", depends_on=["a"]),
        ]
        with pytest.raises(DagValidationError) as exc_info:
            validate_dag(steps)
        assert "Circular" in str(exc_info.value)
        assert set(exc_info.value.cycles) == {"a", "b"}

    def test_self_cycle_raises(self):
        steps = [
            job_step("a", "j1", depends_on=["a"]),
        ]
        with pytest.raises(DagValidationError):
            validate_dag(steps)

    def test_three_node_cycle(self):
        steps = [
            job_step("a", "j1", depends_on=["c"]),
            job_step("b", "j2", depends_on=["a"]),
            job_step("c", "j3", depends_on=["b"]),
        ]
        with pytest.raises(DagValidationError):
            validate_dag(steps)

    def test_complex_valid_dag(self):
        steps = [
            job_step("fetch", "j1"),
            job_step("transform", "j2", depends_on=["fetch"]),
            job_step("validate", "j3", depends_on=["fetch"]),
            job_step("load", "j4", depends_on=["transform", "validate"]),
            job_step("notify", "j5", depends_on=["load"]),
        ]
        result = validate_dag(steps)
        assert len(result) == 5
        assert result[0] == "fetch"
        assert result[-1] == "notify"
