# 18. Prepare unsupported signing PDFs as static page images

Date: 2026-09-05

## Status

Accepted.

## Context and Problem Statement

Signing uploads previously rejected PDFs with forms, annotations, rotation, cropping, or other unsupported structures.
Users need Kosmos to prepare these PDFs automatically without transferring private documents to another service or permitting active document content.

## Considered Options

1. Render unsupported PDFs into a fresh static image-only PDF inside an embedded WebAssembly sandbox.
2. Flatten forms and annotations while preserving vector PDF structures.
3. Prepare PDFs only in the browser.
4. Use a native external converter or document conversion service.

## Decision Outcome

Chosen: **option 1**.

Preserve already supported PDFs byte for byte. Otherwise render each visible page with go-pdfium's embedded PDFium WebAssembly module hosted by wazero, with no filesystem mounts, network access, or document action execution, then build a fresh PDF from page images at 200 DPI with JPEG quality 95. Bound memory, execution time, concurrency, page count, page pixels, and output size. Reject encrypted PDFs, XFA, and unsupported or unrenderable form states rather than silently omitting them. Retain the raw upload privately when conversion occurs. Record uploadedSHA256 for those bytes and originalSHA256 for the prepared signing copy; include both hashes in completion evidence when different. Public signers access only the prepared and completed artifacts. Authenticated workspace users can download the original upload separately.

## Consequences

### Good

- Removes active PDF structures from the signing copy without sending documents to another provider.
- The signer previews exactly the prepared pages on which Kosmos places fields.
- Retains the submitted bytes and a hash chain connecting upload, prepared copy, and completed artifact.
- Keeps the existing stateless deployment and CGO-free Go builds.

### Bad

- Converted document bodies lose selectable text, vector detail, and document-level accessibility structure; the interface must disclose this before signing.
- Raster conversion adds CPU cost, WebAssembly initialization, binary size, and limits that can reject otherwise viewable documents.
- Rendering freezes the visible state, so encrypted, XFA, or unsupported form states require a different input rather than guessing intended content.
- Kosmos must package PDFium and runtime redistribution notices and maintain the embedded renderer alongside the existing PDF library.

### Rejected because

- Vector-preserving flattening retains a broader PDF object surface and requires reliable handling of form appearances, actions, layers, annotations, cropping, and rotation.
- Browser-only conversion lets clients bypass preparation and produces inconsistent output; the server must enforce the signing copy.
- A native converter adds process and deployment dependencies, while an external conversion service transfers sensitive documents outside the current boundary.
