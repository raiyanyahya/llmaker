# llmaker Ollama image: the Ollama engine + the Python control-plane facade.
#
# Build from the repo root so the build context contains ./facade:
#   docker build -f images/ollama/Dockerfile -t ghcr.io/raiyanyahya/llmaker-ollama .
#
# Only the facade port (8080) is exposed; Ollama itself stays on loopback.
FROM ollama/ollama:latest

ENV LLMAKER_BACKEND=ollama \
    FACADE_PORT=8080 \
    OLLAMA_HOST=127.0.0.1:11434 \
    OLLAMA_URL=http://127.0.0.1:11434 \
    PYTHONUNBUFFERED=1 \
    DEBIAN_FRONTEND=noninteractive

# The Ollama base is Ubuntu-based; add Python for the facade.
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pip curl \
 && rm -rf /var/lib/apt/lists/*

# Install the facade package (FastAPI app + static web UI). The `||` fallback
# keeps this working across pip versions: newer pip (PEP 668) needs
# --break-system-packages; older pip rejects the flag but isn't externally managed.
COPY facade/ /opt/llmaker-facade/
RUN python3 -m pip install --no-cache-dir --break-system-packages /opt/llmaker-facade \
 || python3 -m pip install --no-cache-dir /opt/llmaker-facade

COPY images/ollama/entrypoint.sh /usr/local/bin/llmaker-entrypoint
RUN chmod +x /usr/local/bin/llmaker-entrypoint

EXPOSE 8080
VOLUME ["/root/.ollama"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=40s --retries=6 \
  CMD curl -fsS "http://127.0.0.1:${FACADE_PORT}/api/health" || exit 1

# Replace the base image's `ollama` entrypoint with our supervisor script.
ENTRYPOINT ["/usr/local/bin/llmaker-entrypoint"]
CMD []
