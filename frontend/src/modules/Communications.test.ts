import { describe, expect, it } from "vitest";
import { Account, Contact } from "../api";
import { hasTemplateVariables, mergeTemplate } from "./Communications";

describe("email template variables", () => {
  it("merges contact, account, and domain values", () => {
    const contact = { name: "Ada Angler" } as Contact;
    const account = {
      name: "River Labs",
      websites: [
        { url: "https://river.example", domain: "river.example" },
        { url: "https://shop.river.example", domain: "shop.river.example" },
      ],
    } as Account;

    expect(
      mergeTemplate(
        "Hi {{name}} at {{company}}. Domains: {{domains}}.",
        contact,
        account,
      ),
    ).toBe(
      "Hi Ada Angler at River Labs. Domains: river.example, shop.river.example.",
    );
  });

  it("keeps unresolved variables visible until a contact is selected", () => {
    const value = "Hi {{name}} at {{company}}. Domains: {{domains}}.";

    expect(mergeTemplate(value)).toBe(value);
    expect(hasTemplateVariables(mergeTemplate(value))).toBe(true);
    expect(hasTemplateVariables("Hi Ada at River Labs.")).toBe(false);
  });
});
