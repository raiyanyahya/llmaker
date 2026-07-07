from app.config import Settings


def test_from_env_defaults():
    s = Settings.from_env({})
    assert s.backend == "ollama"
    assert s.facade_port == 8080
    assert s.cors_origins == []  # secure default: no cross-origin access
    assert s.api_key == ""


def test_from_env_parses_values():
    s = Settings.from_env(
        {
            "LLMAKER_BACKEND": "LlamaCpp",
            "LLMAKER_NAME": "brave-llama",
            "LLMAKER_DEFAULT_MODEL": "qwen2.5:7b",
            "FACADE_PORT": "9000",
            "API_KEY": "secret",
            "CORS_ORIGINS": "https://a.com, https://b.com",
            "KEEP_ALIVE": "10m",
        }
    )
    assert s.backend == "llamacpp"
    assert s.name == "brave-llama"
    assert s.default_model == "qwen2.5:7b"
    assert s.facade_port == 9000
    assert s.api_key == "secret"
    assert s.cors_origins == ["https://a.com", "https://b.com"]
    assert s.keep_alive == "10m"


def test_from_env_bad_port_falls_back():
    s = Settings.from_env({"FACADE_PORT": "not-a-number"})
    assert s.facade_port == 8080


def test_from_env_empty_cors_disables_cross_origin():
    s = Settings.from_env({"CORS_ORIGINS": ""})
    assert s.cors_origins == []


def test_from_env_wildcard_cors_opts_in():
    s = Settings.from_env({"CORS_ORIGINS": "*"})
    assert s.cors_origins == ["*"]
