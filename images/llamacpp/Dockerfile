# llmaker llama.cpp image (partial / structural): the llama.cpp server + facade.
#
# Build from the repo root:
#   docker build -f images/llamacpp/Dockerfile -t ghcr.io/raiyanyahya/llmaker-llamacpp .
#
# llama.cpp serves a single GGUF, supplied via MODEL_PATH (mount or bake it).
FROM ghcr.io/ggml-org/llama.cpp:server

ENV LLMAKER_BACKEND=llamacpp \
    FACADE_PORT=8080 \
    LLAMACPP_URL=http://127.0.0.1:8081 \
    MODEL_PATH=/models/model.gguf \
    PYTHONUNBUFFERED=1 \
    DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pip curl \
 && rm -rf /var/lib/apt/lists/*

COPY facade/ /opt/llmaker-facade/
RUN python3 -m pip install --no-cache-dir --break-system-packages /opt/llmaker-facade \
 || python3 -m pip install --no-cache-dir /opt/llmaker-facade

COPY images/llamacpp/entrypoint.sh /usr/local/bin/llmaker-entrypoint
RUN chmod +x /usr/local/bin/llmaker-entrypoint

EXPOSE 8080
VOLUME ["/models"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=40s --retries=6 \
  CMD curl -fsS "http://127.0.0.1:${FACADE_PORT}/api/health" || exit 1

ENTRYPOINT ["/usr/local/bin/llmaker-entrypoint"]
CMD []
