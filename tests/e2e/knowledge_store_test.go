package e2e

// T1-10 / SCN-025-01: Concept CRUD via live DB post-migration.
// Knowledge layer tables are created by migration — concept page can be created and retrieved.
//
// Requires: live Docker Compose stack with PostgreSQL + migration 014 applied.
// Run with: ./smackerel.sh test e2e
