# Kosmos

Kosmos is an MIT-licensed, extensible small business management platform. It gives a small business one friendly landing zone for links, relationships, follow-ups, documents, costs, integrations, and notifications.

The application is a Go API with a responsive React frontend, packaged as one stateless Cloud Run service that scales to zero. It includes Google login, roles, accounts and relationship management, Markdown knowledge, reminders, Gmail actions, Google Voice handoffs, Tiller imports, costs and receipts, notifications, audit history, exports, and configurable landing-zone shortcuts. See [plan.md](plan.md) for the product contract.

## Local development

```sh
make dev
```

The API runs on `http://localhost:8080`. The frontend development server runs on `http://localhost:5173`.

Google login requires `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and a random `KOSMOS_SESSION_SECRET` of at least 32 bytes. The same `/auth/callback` handles login and optional incremental Google Workspace authorization. Production also requires `KOSMOS_ALLOWED_GOOGLE_DOMAINS`, a comma-separated list of verified email domains allowed to receive or retain a session. Set `KOSMOS_GCP_PROJECT` to persist data in Firestore, `KOSMOS_ATTACHMENTS_BUCKET` for private files, and `KOSMOS_ORGANIZATION_ID` for the shared organization scope. Without a GCP project, modules use in-memory adapters for local development. `KOSMOS_WEB_ROOT` can point the backend at a local frontend production build.

The production login policy currently permits verified Google accounts at `nerdswhofish.com`, `theoutdoorprogrammer.com`, and `apollorion.com`. This check happens at the callback and on every authenticated request, so removing a domain also invalidates its existing sessions.

## Deployment

`infra/modules/environment` is published to the Spacelift private registry as `kosmos-environment`. Environment roots consume `spacelift.stout.zone/theoutdoorprogrammer/kosmos-environment/google`, while keeping only project bootstrap, operator access, deployment inputs, and secret payloads outside the module.

Every mutating browser request requires the `X-Kosmos-CSRF: 1` header in addition to the signed, HTTP-only session cookie. The versioned API contract lives at [api/openapi.yaml](api/openapi.yaml).

Provider setup, the public contact-form contract, Tiller headers, backup behavior, retention, and recovery are covered in [docs/operations.md](docs/operations.md).

## Observability

The backend accepts standard `OTEL_EXPORTER_OTLP_*` environment variables and exports correlated logs and traces over OTLP HTTP when an endpoint is configured. Without an endpoint, telemetry remains local. Browser RUM and tracing use the public runtime configuration returned by `/api/v1/config` when `KOSMOS_FARO_URL` is present.

## License

MIT. See [LICENSE](LICENSE).
