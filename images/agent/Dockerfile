# llmaker RAG agent image.
#
# A LangGraph retrieval-augmented chat service that wires an llmaker LLM, a
# vector database, and an embeddings endpoint together. It is a plain service on
# the llmaker network; its env defaults point at the conventional in-network
# names (chat, qdrant, embeddings).
#
#   docker build -f images/agent/Dockerfile -t ghcr.io/raiyanyahya/llmaker-agent:latest .
#   llmaker service add agent           # or: llmaker stack init rag && llmaker apply

FROM python:3.12-slim-bookworm

ENV PYTHONUNBUFFERED=1 \
    PORT=8800 \
    PIP_NO_CACHE_DIR=1

RUN apt-get update \
 && apt-get install -y --no-install-recommends curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Install the agent package (FastAPI app + LangGraph pipeline + static web UI).
COPY agent/ /opt/llmaker-agent/
RUN pip install /opt/llmaker-agent

EXPOSE 8800

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=6 \
  CMD curl -fsS "http://127.0.0.1:${PORT}/health" || exit 1

# uvicorn honors $PORT via the app's __main__.
CMD ["python", "-m", "app"]
