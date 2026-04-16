package integration

// T4-06 / BS-003: Email + Maps → CROSS_SOURCE_CONNECTION edge with insight text.
//
// This integration test validates the full cross-source connection flow:
// 1. Ingest email artifact → synthesis → concept "Italian Restaurants" created
// 2. Ingest maps artifact → synthesis → same concept updated with 2nd source type
// 3. Cross-source check triggers → publishes to synthesis.crosssource
// 4. ML sidecar assesses genuine connection → publishes to synthesis.crosssource.result
// 5. Go core creates CROSS_SOURCE_CONNECTION edge with insight text
//
// Requires live stack: Go core + PostgreSQL + NATS + ML sidecar + Ollama.
// Run with: ./smackerel.sh test integration
