# 13. Use standard Markdown and idempotent managed document synchronization

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Repository workflows need to publish documents and attached assets into Kosmos without creating duplicates or leaving stale files. Document bodies should remain portable Markdown instead of depending on new Kosmos-only attachment syntax. Existing documents already use `[[filename]]`, so compatibility must be retained.

## Considered Options

1. Add an idempotent managed-document synchronization endpoint and use standard Markdown image and link syntax
2. Compose the existing document and attachment CRUD endpoints in every workflow
3. Add more custom wiki-style attachment syntax

## Decision Outcome

Kosmos will expose one write-credential-only managed document synchronization endpoint keyed by a caller-provided stable source key. Each request supplies the complete document body, metadata, and desired attachment set. Kosmos creates or updates exactly one document, uploads changed files, removes stale files, and returns the resulting document and attachment metadata. Repeating an identical request is safe.

New document content uses standard Markdown: `![alt](URL)` for images and `[label](URL)` for files. Kosmos may enhance its own authenticated PDF links with an inline preview without changing the stored Markdown. The renderer continues to understand existing `[[filename]]` embeds, but the editor no longer generates that syntax.

## Consequences

### Good

- Workflow retries converge on one document and one attachment set
- Publishers do not need to reproduce partial-failure ordering across several APIs
- Document source remains portable to other Markdown renderers
- Existing custom embeds keep rendering

### Bad

- The synchronization request is larger and requires multipart parsing
- Replace-style reconciliation means callers must always send the complete desired attachment set
- Legacy and standard attachment rendering paths must coexist until old documents are migrated

### Rejected because

- Composing ordinary CRUD requires every publisher to implement duplicate detection, partial-failure recovery, and stale attachment cleanup correctly.
- Additional wiki syntax makes Kosmos documents less portable and creates another parser contract for workflows to learn.
