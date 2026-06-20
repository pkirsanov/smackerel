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

# spec-047 R12.2: upgrade setuptools to fix CVE-2025-47273
# (Path Traversal Vulnerability in setuptools PackageIndex).
# python:3.12-slim ships with setuptools 70.2.0; CVE fix is in 78.1.1+.
# This MUST run AFTER `pip install -r requirements.txt` because some
# pinned packages constrain setuptools transitively; pinning here is
# the explicit final-state pin.
RUN pip install --no-cache-dir --upgrade "setuptools>=78.1.1"

# spec-047 R16: force starlette to ==1.3.1 to fix CVE-2026-48818
# (StaticFiles SSRF on Windows; fix 1.1.0) and CVE-2026-54283
# (multipart DoS; fix 1.3.1). This is a defensive final-state pin that
# matches the explicit starlette==1.3.1 in requirements.txt so the
# patched version is deterministic even if a later step perturbs the
# transitive resolution. The prior `>=0.49.1,<1.0.0` cap is REMOVED:
# it was tied to fastapi 0.115.6 (which capped starlette <0.42.0 and
# broke on the 1.x line). fastapi is now 0.133.0, which permits the
# starlette 1.x line, so the stale `<1.0.0` cap would otherwise force
# starlette back to 0.52.1 and re-expose both CVEs. CVE fix landed in
# 1.3.1; requirements.txt pins the same version.
RUN pip install --no-cache-dir --upgrade "starlette==1.3.1"

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

# spec-047 R14 (BUG-047-002 close-out 2026-05-16): apply Debian Security
# updates to OS packages shipped by the python:3.12-slim base image.
# python:3.12-slim is rebuilt on Python releases, not on Debian Security
# advisories, so OS packages lag by weeks. CVEs addressed at this close-out:
#   CVE-2026-4878  libcap2     1:2.75-10+b8     -> 1:2.75-10+deb13u1
#   CVE-2026-29111 libsystemd0 257.9-1~deb13u1  -> 257.13-1~deb13u1
#   CVE-2026-29111 libudev1    257.9-1~deb13u1  -> 257.13-1~deb13u1
# This step absorbs future Debian Security updates automatically on every
# rebuild; the spec 047 R13 Trivy gate catches anything left behind.
RUN apt-get update \
    && apt-get -y upgrade \
    && rm -rf /var/lib/apt/lists/*

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
