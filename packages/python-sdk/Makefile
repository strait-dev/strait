SHELL := /bin/bash

VENV := .venv
PYTHON := $(VENV)/bin/python
PIP := $(VENV)/bin/pip
RUFF := $(VENV)/bin/ruff
MYPY := $(VENV)/bin/mypy
PYTEST := $(VENV)/bin/pytest

.PHONY: bootstrap lint typecheck test format run-all clean

bootstrap:
	python3 -m venv $(VENV)
	$(PIP) install --upgrade pip
	$(PIP) install -e .[dev]

lint: bootstrap
	$(RUFF) check strait/ tests/

typecheck: bootstrap
	$(MYPY) strait/

test: bootstrap
	$(PYTEST) tests/ -v

format: bootstrap
	$(RUFF) format strait/ tests/

run-all: lint typecheck test

clean:
	rm -rf $(VENV)
