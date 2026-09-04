# Kosmos operations

## Identity and access

Kosmos owns no passwords. Google authenticates users, and production admits only verified identities from the configured domains. The first approved user atomically becomes the organization owner. Later approved users start as members. Owners and administrators can assign owner, admin, member, or read-only viewer roles and can disable access.

Owners and administrators can also create named API credentials for workflows. Kosmos returns each plaintext token once and stores only its SHA-256 hash. Every request rechecks the stored credential, so revocation takes effect immediately. Read-only credentials cannot mutate data. Read-and-write credentials can use ordinary workspace APIs, but neither access level can manage members, credentials, Gmail, Google Voice, Cloudflare, Tiller, or other integrations. Store workflow tokens as secret values such as `KOSMOS_API_TOKEN`, never in repository variables, logs, or Trailwire messages.

Google Workspace access is a separate incremental grant for Gmail compose, Gmail metadata, Gmail send-as settings, and read-only Google Sheets. Refresh tokens are encrypted at rest with `KOSMOS_INTEGRATION_SECRET`, which is distinct from the web-only `KOSMOS_SESSION_SECRET`. The first split-key deployment migrates existing provider tokens from the session key before the web service accepts traffic. Later integration-key rotation intentionally invalidates saved provider tokens and attachment links, so users reconnect Google afterward. Register one OAuth redirect URI at `https://<host>/auth/callback`.

Owners and administrators can map each member to the primary address or an accepted Gmail send-as alias reported by Google. Kosmos rejects arbitrary or unverified addresses and enforces the saved mapping when sending. Existing Google connections must reconnect once to grant the send-as settings scope before aliases can be verified.

The shared Google Voice account is connected separately by an owner or administrator in Settings. This delegated grant may use a different Google identity than the administrator's Kosmos login and requests only Google Contacts access. Add the shared personal Google account as an OAuth test user while the consent screen remains in testing. Kosmos encrypts its refresh token separately from each member's Workspace grant, queues existing contacts after connection, and then mirrors contact creates, edits, and deletes through the private job service. Google Voice reads those Google Contacts; Kosmos does not automate the Voice calling or messaging UI.

## Public contact form

Submit JSON to `POST /api/v1/intake/contact` with `name`, `email`, and optional `company`, `phone`, `message`, and `source`. Include an empty `website` field as the honeypot. Kosmos validates size and email shape, deduplicates by email, records source attribution, and limits each client IP to five attempts per hour per Cloud Run instance. Cloudflare remains the global edge rate-limit boundary.

## Gmail and Tiller

Outbound email always requires an explicit user action and an `Idempotency-Key` header. Retries with the same key return the saved delivery instead of sending twice. Kosmos sends the plain-text body as MIME base64 so transport line limits cannot introduce visible line breaks that were absent from the authored template. Inbox sync stores sender, subject, snippet, time, and thread identifiers only for known contacts. It does not store message bodies or replace Gmail.

## Account event history

Kosmos writes immutable events beneath an account when it can identify the relationship. This includes account and contact edits, opportunity changes, notes, calls, texts, reminders, documents, sent and received email, Cloudflare domains, and Tiller transactions. Account pages show the five newest entries and link to a cursor-paginated history with event-type filters. Google Voice handoffs record that the shared account was opened, not that a call or text was completed.

The timeline starts when this feature is deployed. It does not synthesize older history from mutable source records. External integrations use deterministic event identifiers so retries do not duplicate history. Interactive workspace mutations save the business record first and report timeline-write failures through structured telemetry rather than failing an already completed mutation.

Tiller import expects a header row with `Date`, `Description`, and `Amount`. `Merchant` and `Transaction ID` are optional. The default range is `Transactions!A:Z`. Stable transaction IDs make imports replay-safe. One deterministic contact match is accepted; zero or multiple matches enter the review queue.

Direct Tiller purchases are optional and independent of spreadsheet import. Create a Tiller application webhook for `https://<host>/api/v1/webhooks/tiller`, subscribe it to `order.paid`, and save the one-time `whsec_` signing secret in Kosmos Settings. Map immutable Tiller product IDs to Kosmos accounts there. Kosmos verifies the timestamped HMAC signature, ignores unmapped products, and derives transaction IDs from the event and order-line identity so Tiller retries cannot duplicate revenue.

## Cloudflare domains

Connect a dedicated user API token with Zone Read and Registrar Read access plus the 32-character Cloudflare account ID. Account-owned Cloudflare tokens do not currently support Registrar permissions. Kosmos encrypts the token, reads zones and registrations, and never calls a Cloudflare mutation endpoint. Linking a Cloudflare Registrar domain creates replay-safe reminders 30, 14, and 7 days before its expiry date. A zone registered elsewhere has no registrar expiry in Cloudflare, so the person linking it supplies that date.

Manual Gmail, Google Contacts, and Tiller synchronization requests return HTTP 202 after creating idempotent Cloud Tasks. They never wait for Google APIs in the browser request. Cloud Scheduler queues a synchronization pass at the top of every hour from 9 AM through 5 PM, Monday through Friday, in `America/New_York`. The private `kosmos-jobs` Cloud Run service executes tasks with a dedicated worker identity and a separate OIDC invoker identity. The queue dispatches one job at a time because Google requires mutations against one Contacts account to be serialized. Transactional record creation and effect-derived notification keys make overlapping manual and scheduled work replay-safe. Both web and worker services scale to zero. Investigate failed work in the Cloud Tasks queue first; provider failures return a retryable 5xx response and use bounded exponential backoff.

List endpoints default to 50 records and accept `limit` values through 100 plus the opaque `cursor` returned by the previous response. The browser follows cursors automatically so every record remains reachable without a desktop-only or mobile-only paging workflow.

## Managed documents

Use `PUT /api/v1/managed-documents/{sourceKey}` with a read-and-write API credential to publish one document from an external source. The source key accepts up to 128 letters, numbers, dots, underscores, or hyphens and deterministically identifies the Kosmos document. Send multipart form data with one `document` part containing `{"title":"...","body":"...","links":[]}` and zero or more repeated `files` parts.

The files in each request are the complete desired attachment set. Duplicate basenames are rejected. Changed files are replaced, omitted files are removed, and an identical retry returns the same document without creating another revision. File identifiers are deterministic by source key and basename. Storage and metadata cannot be committed atomically across GCS and Firestore, but every step is replay-safe and a retry converges after a partial provider failure. Publishers should serialize writes for a source key.

Document source uses ordinary Markdown. Images use `![Description](assets/kosmos/logo.svg)` and downloads use `[Download tokens](assets/kosmos/tokens.json)`. Kosmos resolves relative URL-path basenames against attachments on that document and leaves absolute URLs untouched. Existing `[[filename]]` embeds remain readable for compatibility.

## Files, retention, and recovery

Attachments are private objects. Each upload is limited to 10 MB. Documents accept PDF, Markdown, plain text, JSON, CSS, SVG, JPEG, PNG, or WebP. Contact and account photos remain limited to JPEG, PNG, or WebP. Receipt records must link to a cost. Downloads require both an authenticated identity and an application-signed URL that expires after 15 minutes.

Production buckets prevent public access, use uniform access control, retain object versions, and apply lifecycle cleanup. Firestore production enables point-in-time recovery. CSV exports for contacts and costs provide a portable copy before deletion or migration. Restore by selecting the desired Firestore recovery point and object generation, then verify organization ownership before serving traffic.

## Deployment and observability

Publish `infra/modules/environment` to the Spacelift private module registry, then pin that exact tested version in the configurations repository. Never float production to the latest module. Quill is the only application release path. The release workflow tests the application, builds the React bundle, uses GoReleaser to produce Linux binaries, and then assembles, signs, attests, and publishes the image from those artifacts. Spacelift deploys only after the module version is active, the configurations pin is committed, and the complete plan has been reviewed.

Backend tracing uses standard `OTEL_EXPORTER_OTLP_*` variables. The secret value for `OTEL_EXPORTER_OTLP_HEADERS` must use the full environment-header format, including the header name, such as `Authorization=Basic <credential>`. Browser telemetry uses `KOSMOS_FARO_URL`; production accepts only the `https://kosmos.nerdswhofish.com` origin. The managed Grafana dashboard covers request volume, status, p95 latency, backend and frontend errors, and job activity. Grafana alerts on production browser errors, while Cloud Monitoring alerts on public downtime, web and worker 5xx responses, scheduler dispatch failures, and sustained queue backlog. Telemetry must never include tokens, message bodies, receipt contents, or raw customer data. Production verification checks health, anonymous access denial, responsive login behavior, Faro ingestion, Cloud Run logs, asynchronous task execution, and a correlated Grafana trace before accepting the release.
