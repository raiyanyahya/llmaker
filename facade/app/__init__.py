"""llmaker control-plane facade.

A small FastAPI application that runs inside every llmaker container. It wraps a
backend engine (Ollama, llama.cpp, …) and presents one normalized contract:

  * ``/v1/*``           OpenAI-compatible inference (streaming via SSE)
  * ``/api/health``     liveness / readiness
  * ``/api/status``     aggregate instance + system + model status
  * ``/api/models*``    list / pull (streamed) / delete / set-default
  * ``/ws/status``      live status push for the web UI
  * ``/``               a self-contained web UI

Backends are pluggable adapters (``app.adapters``); the routes and UI never know
which engine is running underneath.
"""

__version__ = "0.1.0"
