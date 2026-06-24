#!/usr/bin/env bash
# Supervisor for the llmaker llama.cpp image: start llama-server on loopback with
# the mounted GGUF, then hand PID 1 to the facade.
set -euo pipefail

MODEL_PATH="${MODEL_PATH:-/models/model.gguf}"

if [ ! -f "${MODEL_PATH}" ]; then
  echo "llmaker: no GGUF found at ${MODEL_PATH}." >&2
  echo "Mount one with: llmaker up --backend llamacpp -v /path/to/model.gguf:/models/model.gguf" >&2
fi

# Start the llama.cpp OpenAI-compatible server (internal only).
llama-server --host 127.0.0.1 --port 8081 -m "${MODEL_PATH}" &

# Wait for readiness (bounded).
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:8081/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

exec python3 -m app
