# Kosmos environment

This cohesive module owns web, jobs and their supporting GCP and optional Grafana resources. Publish changes through the private Spacelift module registry and promote an explicitly pinned version in the environment root.

## Changing OTLP destinations

Create and populate a separate same-project Secret Manager container first. Set `otel_exporter_headers_secrets` with both `web` and `jobs` references, each containing its `secret_id` and actual numeric `version`, together with `otel_exporter_otlp_endpoint`. Both processes currently report canonical service name `kosmos`, so they may use the same service credential. The two runtime service accounts receive separate, container-scoped accessor grants.

Existing callers keep the module-managed header container at `latest`. The original container and grants remain for older revisions and rollback. Never replace its latest payload with another destination's credential. Payloads must stay outside OpenTofu configuration and state.

This change preserves application logs, traces and global trace privacy sanitization. It does not add an application metrics provider, alter browser RUM or remove Grafana dashboards, rules or GCP alerts. Retire those resources only after verified replacements and an explicitly reviewed removal plan.
