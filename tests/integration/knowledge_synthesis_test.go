// T2-10 (BS-001): Ingest artifact → synthesis.extract → ML → synthesis.extracted → concept page exists
// T2-11 (BS-002): Ingest 2nd artifact same topic → concept page updated, both citations present
// T2-12 (BS-009): ML sidecar returns failure → artifact has embedding + graph links, synthesis_status=failed
//
// To run: ./smackerel.sh test integration
package integration
