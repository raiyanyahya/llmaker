#!/usr/bin/env bash
# Supervisor for the llmaker Ollama image: start Ollama on loopback, wait for it,
# then hand PID 1 to the facade so signals and container lifecycle behave.
set -euo pipefail

export OLLAMA_HOST="${OLLAMA_HOST:-127.0.0.1:11434}"

# Start the Ollama engine in the background (internal only).
ollama serve &

# Wait until Ollama is accepting requests (bounded), so the facade reports
# healthy promptly once everything is up.
for _ in $(seq 1 60); do
  if curl -fsS "http://${OLLAMA_HOST}/api/tags" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# Optionally warm the default model so the instance is immediately usable even
# if no one calls `llmaker pull` (best-effort; ignore failures).
if [ -n "${LLMAKER_DEFAULT_MODEL:-}" ]; then
  ( ollama pull "${LLMAKER_DEFAULT_MODEL}" >/dev/null 2>&1 || true ) &
fi

# Facade becomes PID 1 (via exec): it serves the OpenAI-compatible API, the
# control plane, and the web UI on FACADE_PORT.
exec python3 -m app
