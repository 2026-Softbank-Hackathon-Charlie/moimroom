# Committed reservation write collection

This optional collector reports committed reservation INSERT primary keys. It
does not certify successful response delivery, all CRUD writes, row contents, or
complete collection of every commit at a migration's verification cutoff.

Configure all three environment variables, or leave all unset:

| Variable | Meaning |
| --- | --- |
| `WRITTEN_RECORDS_URL` | Platform API origin reachable from the application; prefer HTTPS. No URL credentials/query/fragment. |
| `WRITTEN_RECORDS_STREAM` | Dedicated stream, such as `app-moimroom`, up to 63 lowercase letters/digits/hyphens. |
| `WRITTEN_RECORDS_TOKEN` | Random append-only stream token (at least 32 characters), provided through an approved Kubernetes Secret. |

On the platform, configure the matching `PLATFORM_WRITTEN_RECORD_TOKENS` entry
`<stream>:<dedicated token>`. Never use a general operator/agent credential. Do not
put the token in browser bundles, request payloads, logs, Git or this document.
The token cannot query records or start a migration, and cannot claim client
acknowledgement scope. Multiple applications need separate streams/tokens.

Use the approved deployment flow in this order:

1. Deploy the platform schema/API changes. Create a dedicated stream credential
   through the existing secret-management flow.
2. On the current **source primary**, deploy the new application schema containing
   `app.write_collection_identity` and `app.written_record_outbox`; allow normal
   WAL replication to carry both tables/identity to the target and confirm they
   exist there. Keep `MigrateSkip` behavior on the standby. A standby does not
   create these tables itself; enabling collection after promotion without them
   would fail reservation transactions.
3. Enable the collector environment variables on the source application after
   schema readiness, then on the target after replicated schema readiness. Only
   the primary reporter sends data. Use the same stream and generation for the
   same migrating dataset. This document does not perform deployment or writes.
4. Check authenticated platform stream statistics for actual records. `latestAt`
   is the latest recorded event timestamp, not a heartbeat or last arrival. The
   console shows current collection separately from each run's frozen cohort.
5. A draft automatically binds `app-<registered App.name>` only when it fits the
   limit, has recent nonempty application commit receipts and source generation
   matches. Otherwise set `writtenRecordsStream` explicitly in the draft request
   after checking the source. Do not substitute zero for an unavailable result.

The reporter reads batches of 100 every second, with a three-second attempt
timeout including pool/SQL/HTTP. Each sweep has a fixed upper sequence and repeats
from zero on completion, recovering out-of-order commits under continuing traffic.
Central stable event IDs deduplicate replay and retries; redirects are rejected
and full accepted-count acknowledgement is required. The worker only reads SQL,
so it does not bypass gateway write fencing to update delivery state.

The outbox adds an INSERT and indexes per configured booking. Append-only history
and central receipts have no automatic retention or compaction. Monitor storage,
DB read load, replay throughput and reporter retry logs; long sweeps increase
collection lag. A commit may be reported after verification freezes the central
cohort and then remains outside that run. A measured zero means no missing keys
in that fixed, nonempty cohort; it does not prove collection completeness.

Fresh installations get another database generation and must use a new stream
and token. Replication/restore of the same dataset retains generation. Before
manual DELETE/TRUNCATE, reseeding or PK reuse, retire the old stream and rotate
identity/stream during approved maintenance. Do not rotate identity during a
normal migration. Old insert-existence receipts would otherwise mistake a
legitimate deletion for a missing write. Cancellation updates status and retains
the reservation row; all other application operations remain outside this scope.

Tests use `MOIMROOM_TEST_DATABASE_URL` against disposable PostgreSQL; CI supplies
it. Never point this test variable at an operating database: existing integration
fixtures reset the `app` schema.
