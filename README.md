# Kosmos

Kosmos is an MIT-licensed, extensible small business management platform. It gives a small business one friendly landing zone for links, relationships, follow-ups, documents, costs, integrations, and notifications.

The application is a Go API with a responsive React frontend, packaged as one stateless Cloud Run service that scales to zero. The usable foundation includes Google login, shared organization data, contacts, account activity and notes, follow-up reminders, opportunities, Markdown documents, business costs, workspace search, and configurable landing-zone shortcuts. See [plan.md](plan.md) for the product and architecture roadmap.

## Local development

```sh
make dev
```

The API runs on `http://localhost:8080`. The frontend development server runs on `http://localhost:5173`.

Google login requires `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and a random `KOSMOS_SESSION_SECRET` of at least 32 bytes. Production also requires `KOSMOS_ALLOWED_GOOGLE_DOMAINS`, a comma-separated list of verified email domains allowed to receive or retain a session. Set `KOSMOS_GCP_PROJECT` to persist workspace data in Firestore and `KOSMOS_ORGANIZATION_ID` to select the shared organization scope. Without a GCP project, every module uses an in-memory store for local development. `KOSMOS_WEB_ROOT` can point the backend at a local frontend production build.

The production login policy currently permits verified Google accounts at `nerdswhofish.com`, `theoutdoorprogrammer.com`, and `apollorion.com`. This check happens at the callback and on every authenticated request, so removing a domain also invalidates its existing sessions.

## Deployment

`infra/modules/environment` is published to the Spacelift private registry as `kosmos-environment`. Environment roots consume `spacelift.stout.zone/theoutdoorprogrammer/kosmos-environment/google`, while keeping only project bootstrap, operator access, deployment inputs, and secret payloads outside the module.

Every mutating browser request requires the `X-Kosmos-CSRF: 1` header in addition to the signed, HTTP-only session cookie. The versioned API contract lives at [api/openapi.yaml](api/openapi.yaml).

## Observability

The backend accepts standard `OTEL_EXPORTER_OTLP_*` environment variables and exports correlated logs and traces over OTLP HTTP when an endpoint is configured. Without an endpoint, telemetry remains local. Browser RUM and tracing use the public runtime configuration returned by `/api/v1/config` when `KOSMOS_FARO_URL` is present.

## License

MIT. See [LICENSE](LICENSE).
