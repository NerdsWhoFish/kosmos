# 2. Restrict production login by verified Google email domain

Date: 2026-09-03

## Status

Accepted.

## Context and Problem Statement

Kosmos contains private customer, financial, and operational data. It uses Google OIDC so the application must decide which valid Google identities may receive a Kosmos session.

## Considered Options

1. Allow only verified Google identities from configured email domains
2. Maintain an exact email address allowlist
3. Allow any valid Google identity

## Decision Outcome

The OAuth callback accepts an identity only when Google marks the email as verified and its normalized domain exactly matches a configured allowlist. Production configures `nerdswhofish.com`, `theoutdoorprogrammer.com`, and `apollorion.com`. The backend enforces the policy before creating a session, and IaC owns the production domain list.

## Consequences

### Good

- Kosmos does not manage passwords or a second identity store.
- New members of an approved business domain can use Kosmos without an application deployment.
- Authorization remains server-side and applies to every private API.

### Bad

- Any compromised account in an approved domain can reach Kosmos.
- Removing one individual while keeping their Google account active requires a narrower future deny or membership policy.
- Production access depends on the security and lifecycle controls of all three Google domains.

### Rejected because

- An exact email allowlist has tighter membership control but creates manual application configuration churn for every joiner and leaver.
- Allowing any Google identity does not protect private business data and was rejected.
