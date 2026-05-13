# User Validation: ML Sidecar Health Isolation

## Checklist

- [x] Planning packet covers health-route isolation from CPU-bound embedding work.
- [x] Planning packet covers explicit bounded worker concurrency.
- [x] Planning packet requires health latency proof under load.
- [x] Planning packet requires worker and queue observability.
- [x] Implementation isolates `/health` from CPU-bound embedding via a dedicated `ThreadPoolExecutor` (`ml/app/embedder.py::_ensure_executor`).
- [x] Worker concurrency is bound by the SST key `services.ml.embedding_workers` (no hardcoded defaults; sidecar refuses to start if missing/invalid).
- [x] Queue depth is bound by the SST key `services.ml.embedding_queue_max`; excess work is rejected with a named `RuntimeError`.
- [x] Health responsiveness under load is proven by `test_spec050_health_handler_unblocked_by_busy_executor` against the configured `ML_HEALTH_LATENCY_SLA_MS` budget.
- [x] Observability is provided via three Prometheus metrics (`smackerel_ml_embedding_workers_configured`, `smackerel_ml_embedding_inflight`, `smackerel_ml_embedding_rejected_total`).
- [x] `docs/Operations.md` documents the SST keys and tuning guidance.

