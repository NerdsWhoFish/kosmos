# 9. Use private photo uploads instead of importing LinkedIn profile photos

Date: 2026-09-04

## Status

Accepted.

## Context and Problem Statement

Kosmos contacts can link to a LinkedIn profile and benefit from a recognizable photo. LinkedIn's official Profile API limits access to approved applications and says applications may never store profile data for members other than the authenticated member. Scraping arbitrary profile photos would violate those constraints and depend on unstable private page markup.

## Considered Options

1. Allow private image uploads for contacts and keep LinkedIn as an outbound profile link
2. Scrape arbitrary LinkedIn profile pages for photo URLs
3. Use LinkedIn's Profile API to import arbitrary contacts

## Decision Outcome

Kosmos will accept private, user-uploaded contact photos through its existing attachment storage. LinkedIn URLs remain clickable context only. Kosmos will add automatic LinkedIn photos only if LinkedIn later provides an approved API that explicitly permits retrieving and storing other members' images.

## Consequences

### Good

- Contact photos remain under the organization's control and use the existing private storage boundary
- The integration complies with LinkedIn's published data-storage restriction
- Kosmos does not depend on fragile LinkedIn page markup or authentication cookies

### Bad

- Users must upload contact photos manually
- Photos can become stale because Kosmos does not synchronize LinkedIn changes
- A profile link alone cannot automatically supply a visual identity

### Rejected because

- Scraping violates platform expectations, risks account enforcement, and is operationally brittle
- The official API does not provide a compliant general-purpose lookup and storage path for arbitrary contacts
