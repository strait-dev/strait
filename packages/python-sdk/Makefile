SHELL := /bin/bash

VENV := .venv
PYTHON := $(VENV)/bin/python
PIP := $(VENV)/bin/pip
RUFF := $(VENV)/bin/ruff
MYPY := $(VENV)/bin/mypy

.PHONY: bootstrap lint typecheck run-all clean

bootstrap:
	python3 -m venv $(VENV)
	$(PIP) install --upgrade pip
	$(PIP) install -e .[dev]

lint: bootstrap
	$(RUFF) check strait

typecheck: bootstrap
	$(MYPY) strait

run-all: lint typecheck

clean:
	rm -rf $(VENV)
