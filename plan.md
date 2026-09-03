# Kosmos implementation plan

Kosmos is an MIT-licensed, API-first small business management platform for people who need a clear home base for their business without learning an ERP.

## Product principles

- Friendly first: the primary screen should answer "what needs my attention?" immediately.
- API first: every user-visible capability has a versioned REST contract.
- Extensible by composition: new capabilities are modules, not edits scattered through the core.
- Cloud-native: stateless services on Cloud Run, scale-to-zero by default, durable state behind explicit adapters.
- Safe defaults: least-privilege roles, secure cookies, audit history, no secrets or customer PII in telemetry.

## Architecture

### Repository layout

```text
api/
backend/
  cmd/kosmos/
  internal/platform/
  internal/modules/
frontend/
  src/app/
  src/components/
  src/modules/
infra/
  tofu/
  cloudflare/
  observability/
```

The backend is written in Go. The frontend is React with TypeScript. The public application is initially packaged as one stateless Cloud Run service so the browser shell and API share a deployment boundary, while the code remains split into independently testable frontend and backend modules.

Kosmos does not own passwords. Users authenticate through Google OAuth/OIDC, then Kosmos maps the verified Google identity to organization membership and roles. Password reset, MFA, suspicious-login detection, and account recovery remain Google responsibilities.

## Product capabilities

### Landing Zone

The landing zone is the first screen and the product's organizing principle. It provides:

- configurable buttons for websites, Google tools, bookings, payment links, documents, and any future URL
- a quick-add menu for the most common actions
- an at-a-glance summary of follow-ups, open opportunities, costs, and recent activity
- one notification feed for events from every module
- safe defaults and plain-language labels for nontechnical users

Buttons and dashboard cards are registered by modules, so adding a capability does not require rewriting the shell.

### Documents and knowledge

- Markdown documents are authored in the browser and rendered when not editing.
- Documents have stable IDs, revision history, attachments, and typed links to platform records.
- Internal links resolve through stable route identifiers, not database implementation details.
- Documents can be linked from contacts, opportunities, activities, costs, and landing-zone buttons.

### Relationship management

- Leads, contacts, and complete prospect/customer accounts.
- Opportunities with configurable pipeline stages, amounts, owners, and close dates.
- Activities and notes on a unified account timeline.
- Follow-up reminders with due dates, ownership, completion state, and landing-zone notifications.
- Contact-form ingestion with validation, spam controls, deduplication, and source attribution.
- Search and filtering across relationship records.

### Email and communications

- Google OAuth connection tied to an individual Kosmos user.
- Explicit outbound email action for prospecting or customer communication.
- Reusable templates with merge fields, previews, and an audit record of the sender.
- Notifications when a connected Gmail account receives relevant prospect/customer mail metadata.
- Google Voice notification links, which open the relevant conversation or call in Google Voice.
- No full mailbox replacement, deep Google Voice control, or telephony platform in the first release.

### Identity, access, and files

- User accounts, organization membership, and roles.
- Permission names are module-owned and checked at the API boundary.
- Secure sessions, CSRF protection for browser mutations, rate limits, and audit history.
- File attachments backed by object storage with signed, expiring access URLs.
- Receipt uploads retain source metadata and link to a cost record.

### Costs and lightweight ERP

- One-time and recurring business costs.
- Subscriptions, registration costs, renewals, vendors, categories, tax notes, and payment methods.
- Receipt attachment and retention metadata.
- Review state for incomplete or uncertain records.
- Exportable records for accounting and tax preparation.

### Tiller integration

- Import Tiller transactions through a provider adapter.
- Idempotent transaction ingestion and import history.
- Deterministic matching against customers, vendors, and known payment descriptors.
- A review queue for ambiguous matches, never silent reassignment.
- Optional link between a transaction and an account, opportunity, cost, or purchase event.

### Notifications and activity feed

All modules publish normalized events such as a new contact-form submission, inbound email notification, completed purchase, imported transaction, new reminder, or changed opportunity. The platform stores an idempotent notification projection for the landing zone, with links back to the source record and module.

## Extensibility model

Kosmos is intentionally a platform, not a fixed CRM. New functionality must be addable as a module with a predictable contract.

### Backend module contract

Each module owns its domain types, application services, persistence ports, HTTP routes, authorization policy, event handlers, navigation metadata, and tests. The platform registry lets modules register:

- versioned API routes and resources
- landing-zone buttons, dashboard cards, and quick actions
- notification and activity event types
- permission names and role defaults
- background jobs and integration adapters
- search providers and document link targets

The core shell depends on module interfaces, never on concrete CRM, finance, or integration implementations.

### Frontend module contract

Each frontend module owns its routes, screens, API client, empty/loading/error states, permission-aware actions, and navigation metadata. Shared UI primitives live in `frontend/src/components`. The shell consumes module metadata instead of maintaining a hand-written list of every feature.

### Cross-module rules

1. No feature writes another module's tables directly.
2. No cross-module UI link depends on an internal database ID without a route resolver.
3. New integrations implement an adapter port and normalize provider events at the boundary.
4. API changes are additive within `/api/v1`; breaking changes require a new version.
5. Every durable event has an idempotency key and a replay-safe handler.
6. Every new module ships its permission list, navigation metadata, API contract, migrations, and tests together.
7. Configuration comes from typed environment-backed settings, not package-level globals.

## API and data contracts

- REST API is the source of truth for browser and future clients.
- OpenAPI is versioned in `api/openapi.yaml` and checked in CI.
- API responses use stable envelopes, explicit error codes, pagination, and arrays instead of JSON `null` for empty lists.
- Mutating requests accept idempotency keys where retries can create records or send messages.
- Webhook endpoints authenticate providers, validate signatures, record delivery IDs, and acknowledge only after durable processing.
- Database migrations are forward-only and run as a separately controlled deployment step.

Core records are accounts, contacts, leads, opportunities, pipeline stages, activities, reminders, documents, notifications, attachments, costs, receipts, imported transactions, integration connections, and audit entries. Stable IDs and organization ownership are present from the first persistent schema.

## Infrastructure as code

All infrastructure is managed with OpenTofu. No production resource may require a click in a console after the initial provider and billing bootstrap.

The reusable deployment module lives at `infra/tofu/modules/kosmos-cloud-run`. The `TheOutdoorProgrammer/configurations` repository will call this module from a Spacelift-managed stack, supply the project, environment, immutable image digest, secrets, and edge settings, and own environment composition. Kosmos does not embed credentials or assume a particular configurations repository layout.

### GCP resources

The OpenTofu stack provisions, per environment:

- project APIs and required service enablement
- Artifact Registry repository for immutable Kosmos images
- Cloud Run service with minimum instances set to zero by default
- dedicated runtime service account with least-privilege roles
- Secret Manager secrets and version references for OAuth, session signing, database, storage, Tiller, and telemetry credentials
- Cloud Storage bucket for attachments and receipts, with uniform bucket-level access, retention, lifecycle rules, and public access prevention
- Firestore Native mode database, indexes, backup policy, and retention configuration for the near-free default
- optional Postgres-compatible database module for deployments that need relational reporting beyond Firestore's model
- Cloud Tasks or Pub/Sub for retries and work that cannot depend on a live request
- Cloud Scheduler only for explicitly required periodic jobs
- Artifact deploy policy, revision labels, startup/readiness configuration, and rollback-safe traffic management
- log-based metrics, uptime checks, alert policies, and notification channels

The first reusable module intentionally provisions the Cloud Run application boundary, Artifact Registry, runtime identity, required APIs, secret access bindings, and public invocation policy. Firestore, storage, queue, Cloudflare, and Grafana resources remain separate modules so configurations can compose only what an environment needs and keep idle cost near zero.

Cloud Run is stateless. Background work uses a queue because scale-to-zero instances do not run continuously. The web service must not depend on local disk for user data.

### Cloudflare resources

Cloudflare configuration is also managed as code:

- tunnel and connector configuration for `cast.nerdswhofish.com` or the eventual Kosmos hostname
- DNS records and proxied routing
- Access policies for administrative or staging surfaces
- rate limiting, WAF rules, and security headers where appropriate
- origin rules that prevent bypassing the intended edge boundary

Cloudflare API tokens are supplied through the local secret manager or CI secret store, never committed to the repository.

### Observability resources

Grafana Cloud dashboards, alert rules, data source configuration, and Faro application configuration are represented in `infra/observability` wherever the provider API supports it. The production stack is Grafana Cloud stack `1807923` in `prod-us-east-3`.

## Observability and operations

- Go uses OpenTelemetry tracing and OTLP HTTP/protobuf export.
- HTTP requests, datastore calls, queue jobs, provider calls, and errors carry spans.
- W3C trace context is propagated across HTTP and asynchronous jobs.
- Structured logs include trace and span IDs when a span exists.
- The browser has one Grafana Faro initialization boundary for RUM, frontend errors, and performance signals.
- Telemetry attributes are stable and low-cardinality. Never emit secrets, email bodies, receipt contents, or raw customer PII.
- Dashboards cover request rate, latency, errors, queue age, provider failures, database health, storage failures, and Cloud Run instance behavior.
- Alerts are actionable, deduplicated, and tied to a runbook.
- Production verification includes a real trace and correlated log in Grafana before launch.

## Security and privacy

- Google OAuth scopes are minimal and separated by capability.
- Organization and role checks happen server-side for every protected resource.
- Webhook signatures, CSRF defenses, input validation, content-size limits, and upload type checks are mandatory.
- Attachments are private by default and served only through short-lived signed URLs.
- Email sending requires an explicit user action and records the connected sender.
- Audit history records security-sensitive and externally visible changes.
- Backups, retention, deletion, and export paths are documented before production data is accepted.

## Delivery slices

1. Foundation: landing zone shell, API registry, health endpoint, OTel plumbing, local development, Cloud Run container, reusable OpenTofu module.
2. Identity and shell: Google OAuth/OIDC sign-in, organizations, roles, module navigation, notification feed.
3. Relationship management: contacts, leads, opportunities, pipeline stages, activities, notes, reminders, and account timeline.
4. Documents and files: Markdown documents, stable internal links, rendering, attachment storage, receipt uploads.
5. Google workspace: linked Gmail account, templates, explicit outbound cold-email action, inbound email notification metadata.
6. Money and integrations: costs, recurring costs, tax receipt records, Tiller import, deterministic customer matching with review queue.
7. Hardening: audit history, rate limits, job retries, exports, backups, accessibility, and tenant isolation tests.

Every delivery slice requires backend tests and frontend tests. A release cannot be tagged or published until both suites and the production frontend build pass.

Releases use Quill through `.github/workflows/release.yml`. Quill owns versioning, tagging, image publication, signing, and provenance. GitHub OIDC supplies short-lived Artifact Registry access through a release service account; no service-account key is stored in GitHub.

## Deliberate non-goals for the first release

- Full Gmail replacement or mailbox synchronization.
- Twilio-style telephony or Google Voice control.
- A generic automation builder before the event and job contracts prove what is needed.
- Payroll, inventory accounting, or tax filing.
