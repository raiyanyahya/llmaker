"""Shared pytest fixtures for the facade tests."""

from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from app.config import Settings
from app.main import create_app
from fake_adapter import FakeAdapter


@pytest.fixture
def adapter() -> FakeAdapter:
    return FakeAdapter()


@pytest.fixture
def settings() -> Settings:
    return Settings(name="test-inst", backend="fake", default_model="llama3:8b")


@pytest.fixture
def client(settings: Settings, adapter: FakeAdapter):
    app = create_app(settings=settings, adapter=adapter)
    with TestClient(app) as c:
        yield c
