# Kosmos delivery plan

This checklist is the release source of truth. Check an item only after its backend, frontend, API contract, tests, and responsive behavior are complete where applicable.

- [x] 1. Make landing-zone links organization-wide. Owners and admins configure them once on Overview, and every member sees the same links without setup.
- [x] 2. Remove the redundant relationship count and second add-contact prompt from the bottom of Overview.
- [x] 3. Show open, won, and lost opportunity counts and amounts on Overview.
- [x] 4. Show Google Voice call and message actions on each contact screen.
- [x] 5. Make opportunities belong to accounts, with a contact remaining optional context rather than the owner of the opportunity.
- [x] 6. Support desktop drag and drop between opportunity pipeline stages, with an accessible non-drag fallback that also works on mobile.
- [x] 7. Open the linked account when an opportunity is selected.
- [x] 8. Create Kosmos documents directly from an account and show that account's documents newest first.
- [x] 9. Split Opportunities into Pipeline, Won, and Lost tabs. Keep won and lost out of pipeline columns, allow desktop drops onto Won and Lost, and sort completed lists from most recent to oldest.
- [x] 10. Repair and polish the Inbox layout on desktop and mobile.
- [x] 11. Let Google Voice actions choose an existing contact or accept a manually entered phone number.
- [x] 12. Show the supported email-template variables where templates are written.
- [x] 13. Add an optional authenticated Tiller-to-Kosmos purchase webhook and admin product mapping so a purchased Tiller product records its transaction against a configured Kosmos account without requiring spreadsheet import.
- [x] 14. Let owners and admins map each member's Google login to a verified Gmail send-as address, and send Kosmos email through that configured alias.
- [x] 15. Support multiple websites when creating an account.
- [x] 16. Optionally create the first contact atomically while creating an account.
- [x] 17. Remove duplicate business-name fields from contacts and use the linked account as the business identity.
- [x] 18. Keep prospect/customer lifecycle state on the account and opportunity, not the contact.
- [x] 19. Add a read-only Cloudflare integration that links a domain to an account and creates idempotent renewal reminders 30, 14, and 7 days out. Cloudflare Registrar supplies automatic renewal dates; externally registered zones require a manual renewal date.
- [x] 20. Let contacts store, edit, and open that person's LinkedIn profile.
- [x] Run backend unit, race, vet, API-contract, and integration tests.
- [x] Run frontend unit, responsive desktop/mobile, accessibility, and production-build tests.
- [ ] Release with Quill and deploy the complete checklist to production.
