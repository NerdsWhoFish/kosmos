# Kosmos delivery plan

This checklist is the release source of truth. Check an item only after its backend, frontend, API contract, tests, responsive behavior, and production verification are complete where applicable.

- [ ] 1. Let owners and administrators edit and delete organization-wide landing-zone shortcuts.
- [ ] 2. Let users delete an errant opportunity after explicit confirmation.
- [ ] 3. Add comfortable separation between an account's relationship panels and Knowledge section.
- [ ] 4. Add a dedicated, authenticated, mobile-first `/lead` intake flow optimized for fast entry at events.
- [ ] 5. Make contact source an organization-wide picklist with an inline create-new-source flow.
- [ ] 6. Make document creation and editing a full-width workspace on phones, tablets, and wide screens.
- [ ] 7. Add Markdown syntax highlighting and line numbers to the document editor.
- [ ] 8. Restyle account and contact document rows so they use the active theme instead of browser-default gray.
- [ ] 9. Let users delete documents after explicit confirmation.
- [ ] 10. Support `{{domains}}` in email templates and document the variable in the template UI.
- [ ] 11. Add a Chrome and Safari Kosmos Companion extension that opens Google Voice with a contact's number prepared, plus in-app extension detection and installation guidance.
- [ ] 12. Display the signed-in user's Google profile picture with an initials fallback.
- [ ] 13. Let users upload files directly to Documents, download them, and embed attached images and PDFs with `[[filename]]` Markdown references.
- [ ] 14. Use a contact's LinkedIn profile photo when LinkedIn provides a compliant, reliable way to retrieve and store it.
- [ ] 15. Let users upload and display account and contact photos.
- [ ] 16. Diagnose and repair the Sender addresses `502`, with useful user-facing errors and production verification.
- [ ] 17. Let users attach a receipt while recording a business cost and show its download link with the cost.
- [ ] 18. Let users edit existing accounts, including adding, editing, and removing the domains tied to an account.
- [ ] Run backend unit, race, vet, API-contract, and integration tests.
- [ ] Run frontend unit, responsive phone/tablet/desktop, accessibility, and production-build tests.
- [ ] Release with Quill and deploy the complete checklist to production through the pinned Spacelift module.
