# 1. Publish one environment module per Kosmos deployment

Date: 2026-09-03

## Status

Accepted.

## Context and Problem Statement

Kosmos infrastructure was split across Cloud Run, Firestore, and storage modules, leaving each deployment root to recreate application wiring, security, secrets, cost controls, and operational defaults. Tiller already proves the desired estate pattern: publish one application environment module to Spacelift and consume it from a thin configurations root.

## Considered Options

1. Publish one cohesive Kosmos environment module
2. Keep three low-level modules and compose them in every environment root
3. Put all Kosmos infrastructure directly in the configurations repository

## Decision Outcome

Chosen: **option 1**.

Publish `infra/modules/environment` as `kosmos-environment` in the Spacelift private module registry. The module owns the reusable application topology and safe defaults. The configurations root owns only environment bootstrap concerns such as the GCP project, operator access, deployment inputs, and secret payload population.

## Consequences

### Good

- Deployment roots consume the same tested topology and defaults.
- Module releases and application releases use the same tag-driven Spacelift and Quill workflow as Tiller.
- Security, scale-to-zero, budget, storage, data, and release identity changes are reviewed together.

### Bad

- Consumers cannot independently swap out one managed service without changing the environment module contract.
- A module release has a wider infrastructure blast radius and requires stronger tests.
- Bootstrap still requires the Spacelift GitHub App to access the NerdsWhoFish repository.

### Rejected because

- Three low-level modules make every consumer responsible for application-specific wiring and allow defaults to drift.
- Direct configuration code is not reusable or independently versioned through the Spacelift module registry.
