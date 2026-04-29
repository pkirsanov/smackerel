# syntax=docker/dockerfile:1

# --- Build stage: install dependencies with CPU-only PyTorch ---
FROM python:3.12-slim AS builder

WORKDIR /app

ENV HF_HOME=/opt/hf-cache/huggingface
ENV SENTENCE_TRANSFORMERS_HOME=/opt/hf-cache/sentence-transformers

# Install torch CPU-only first to avoid pulling CUDA dependencies
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Preload the embedding model during the image build so live processing does
# not depend on a first-request network/cache write under the non-root user.
RUN mkdir -p "$HF_HOME" "$SENTENCE_TRANSFORMERS_HOME" \
    && python -c "from sentence_transformers import SentenceTransformer; SentenceTransformer('all-MiniLM-L6-v2')"

# Strip __pycache__, test files, and type stubs to reduce image size.
# Keep package metadata: transformers validates dependencies via importlib.metadata.
RUN find /usr/local/lib/python3.12/site-packages \
    -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null; \
    find /usr/local/lib/python3.12/site-packages \
    -type d -name "tests" -exec rm -rf {} + 2>/dev/null; \
    find /usr/local/lib/python3.12/site-packages \
    -name "*.pyc" -delete 2>/dev/null; \
    true

# --- Runtime stage ---
FROM python:3.12-slim

ARG VERSION=dev
ARG COMMIT_HASH=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT_HASH}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.title="smackerel-ml"
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"

WORKDIR /app

ENV HOME=/home/smackerel
ENV HF_HOME=/home/smackerel/.cache/huggingface
ENV SENTENCE_TRANSFORMERS_HOME=/home/smackerel/.cache/sentence-transformers

COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY app/ app/

# SEC-002: Run as non-root user
RUN groupadd -r smackerel && useradd -r -g smackerel -d /home/smackerel -m smackerel
COPY --from=builder --chown=smackerel:smackerel /opt/hf-cache /home/smackerel/.cache
USER smackerel

EXPOSE 8081

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8081"]
