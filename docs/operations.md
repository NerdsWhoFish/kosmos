# Kosmos operations

## Identity and access

Kosmos owns no passwords. Google authenticates users, and production admits only verified identities from the configured domains. The first approved user atomically becomes the organization owner. Later approved users start as members. Owners and administrators can assign owner, admin, member, or read-only viewer roles and can disable access.

Google Workspace access is a separate incremental grant for Gmail compose, Gmail metadata, and read-only Google Sheets. Refresh tokens are encrypted at rest with `KOSMOS_INTEGRATION_SECRET`, which is distinct from the web-only `KOSMOS_SESSION_SECRET`. The first split-key deployment migrates existing provider tokens from the session key before the web service accepts traffic. Later integration-key rotation intentionally invalidates saved provider tokens and attachment links, so users reconnect Google afterward. Register one OAuth redirect URI at `https://<host>/auth/callback`.

## Public contact form

Submit JSON to `POST /api/v1/intake/contact` with `name`, `email`, and optional `company`, `phone`, `message`, and `source`. Include an empty `website` field as the honeypot. Kosmos validates size and email shape, deduplicates by email, records source attribution, and limits each client IP to five attempts per hour per Cloud Run instance. Cloudflare remains the global edge rate-limit boundary.

## Gmail and Tiller

Outbound email always requires an explicit user action and an `Idempotency-Key` header. Retries with the same key return the saved delivery instead of sending twice. Inbox sync stores sender, subject, snippet, time, and thread identifiers only for known contacts. It does not store message bodies or replace Gmail.

Tiller import expects a header row with `Date`, `Description`, and `Amount`. `Merchant` and `Transaction ID` are optional. The default range is `Transactions!A:Z`. Stable transaction IDs make imports replay-safe. One deterministic contact match is accepted; zero or multiple matches enter the review queue.

Manual Gmail and Tiller synchronization requests return HTTP 202 after creating an idempotent Cloud Task. They never wait for Google APIs in the browser request. Cloud Scheduler queues a synchronization pass at the top of every hour from 9 AM through 5 PM, Monday through Friday, in `America/New_York`. The private `kosmos-jobs` Cloud Run service executes tasks with a dedicated worker identity and a separate OIDC invoker identity. Transactional record creation and effect-derived notification keys make overlapping manual and scheduled work replay-safe. Both web and worker services scale to zero. Investigate failed work in the Cloud Tasks queue first; provider failures return a retryable 5xx response and use bounded exponential backoff.

List endpoints default to 50 records and accept `limit` values through 100 plus the opaque `cursor` returned by the previous response. The browser follows cursors automatically so every record remains reachable without a desktop-only or mobile-only paging workflow.

## Files, retention, and recovery

Attachments are private objects. Uploads are limited to 10 MB and PDF, text, JPEG, PNG, or WebP. Receipt records must link to a cost. Downloads require both an authenticated session and an application-signed URL that expires after 15 minutes.

Production buckets prevent public access, use uniform access control, retain object versions, and apply lifecycle cleanup. Firestore production enables point-in-time recovery. CSV exports for contacts and costs provide a portable copy before deletion or migration. Restore by selecting the desired Firestore recovery point and object generation, then verify organization ownership before serving traffic.

## Deployment and observability

Publish `infra/modules/environment` to the Spacelift private module registry, then consume the latest active module version from the configurations repository. The production stack intentionally floats within the private registry, matching Tiller's release model, so a deployment must review the resolved module version and complete plan before approval. Quill is the only application release path. It versions, signs, and publishes the application image before Spacelift deploys it.

Backend tracing uses standard `OTEL_EXPORTER_OTLP_*` variables. The secret value for `OTEL_EXPORTER_OTLP_HEADERS` must use the full environment-header format, including the header name, such as `Authorization=Basic <credential>`. Browser telemetry uses `KOSMOS_FARO_URL`; production accepts only the `https://cast.nerdswhofish.com` origin. The managed Grafana dashboard covers request volume, status, p95 latency, backend and frontend errors, and job activity. Grafana alerts on production browser errors, while Cloud Monitoring alerts on public downtime, web and worker 5xx responses, scheduler dispatch failures, and sustained queue backlog. Telemetry must never include tokens, message bodies, receipt contents, or raw customer data. Production verification checks health, anonymous access denial, responsive login behavior, Faro ingestion, Cloud Run logs, asynchronous task execution, and a correlated Grafana trace before accepting the release.
