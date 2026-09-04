import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";

const user = {
  email: "joey@nerdswhofish.com",
  name: "Joey Stout",
  picture: "https://images.example/joey.jpg",
};
const account = {
  id: "account-1",
  name: "River Labs",
  website: "",
  websites: [
    { url: "https://river.example", domain: "river.example", autoRenew: false },
  ],
  billingEmail: "",
  status: "prospect",
  notes: "",
  createdAt: "2026-09-03T12:00:00Z",
  updatedAt: "2026-09-03T12:00:00Z",
};
const contact = {
  id: "contact-1",
  accountId: account.id,
  name: "Ada Angler",
  email: "ada@example.com",
  phone: "+15551234567",
  linkedinUrl: "https://www.linkedin.com/in/ada-angler",
  source: "referral",
  createdAt: "2026-09-03T12:00:00Z",
  updatedAt: "2026-09-03T12:00:00Z",
};
const responses: Record<string, unknown> = {
  "/api/v1/summary": {
    contacts: 1,
    openOpportunities: 1,
    pipelineAmountCents: 125000,
    wonOpportunities: 2,
    wonAmountCents: 500000,
    lostOpportunities: 1,
    lostAmountCents: 75000,
    followUpsDue: 1,
    currentMonthCostCents: 1800,
    recentActivities: [],
  },
  "/api/v1/landing": {
    buttons: [
      {
        id: "docs",
        label: "Field notes",
        description: "Open the handbook.",
        href: "/documents",
        icon: "globe",
      },
    ],
    notifications: [],
  },
  "/api/v1/contacts": { contacts: [contact] },
  "/api/v1/contact-sources": {
    sources: [
      { id: "event", name: "Event", createdAt: "", updatedAt: "" },
      { id: "referral", name: "Referral", createdAt: "", updatedAt: "" },
    ],
  },
  "/api/v1/accounts": { accounts: [account] },
  "/api/v1/accounts/account-1": {
    account,
    contacts: [contact],
    opportunities: [],
    documents: [],
  },
  "/api/v1/opportunities": {
    opportunities: [
      {
        id: "opportunity-1",
        name: "Website refresh",
        accountId: account.id,
        contactId: contact.id,
        amountCents: 125000,
        stage: "qualified",
        nextStep: "Send proposal",
        closeDate: "",
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/activities": { activities: [] },
  "/api/v1/reminders": {
    reminders: [
      {
        id: "reminder-1",
        contactId: contact.id,
        title: "Send proposal",
        dueAt: "2026-09-03T12:00:00Z",
        completed: false,
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/documents": {
    documents: [
      {
        id: "document-1",
        title: "Client kickoff",
        body: "# Agenda",
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/costs": {
    costs: [
      {
        id: "cost-1",
        vendor: "Google",
        description: "Workspace",
        amountCents: 1800,
        category: "Software",
        incurredOn: "2026-09-03",
        recurring: true,
        recurrence: "monthly",
        taxDeductible: true,
        notes: "",
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/search?q=river": {
    results: [
      {
        id: contact.id,
        kind: "contact",
        title: contact.name,
        subtitle: account.name,
        href: `/contacts/${contact.id}`,
      },
    ],
  },
  "/api/v1/members": {
    members: [
      {
        id: "member-1",
        email: user.email,
        name: user.name,
        role: "owner",
        status: "active",
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/pipeline-stages": {
    stages: [
      {
        id: "new",
        name: "New",
        position: 0,
        probability: 10,
        closed: false,
        won: false,
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
      {
        id: "qualified",
        name: "Qualified",
        position: 1,
        probability: 30,
        closed: false,
        won: false,
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
      {
        id: "won",
        name: "Won",
        position: 2,
        probability: 100,
        closed: true,
        won: true,
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
      {
        id: "lost",
        name: "Lost",
        position: 3,
        probability: 0,
        closed: true,
        won: false,
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/audit": { entries: [] },
  "/api/v1/integrations/google": {
    connected: true,
    connection: {
      id: "google-1",
      userEmail: user.email,
      googleEmail: user.email,
      tiller: { spreadsheetId: "sheet-1", range: "Transactions!A:Z" },
      createdAt: "2026-09-03T12:00:00Z",
      updatedAt: "2026-09-03T12:00:00Z",
    },
    connectUrl: "/auth/connect/workspace",
  },
  "/api/v1/integrations/google-contacts": {
    connected: true,
    googleEmail: "shared.voice@gmail.com",
    connectUrl: "/auth/connect/voice-contacts",
    pending: 0,
    failed: 0,
    synced: 1,
  },
  "/api/v1/integrations/cloudflare": { connected: false },
  "/api/v1/email/send-as": { mappings: [] },
  "/api/v1/integrations/tiller/webhook": {
    connected: false,
    endpoint: "/api/v1/webhooks/tiller",
  },
  "/api/v1/integrations/tiller/product-mappings": { mappings: [] },
  "/api/v1/email/templates": {
    templates: [
      {
        id: "template-1",
        name: "Welcome note",
        subject: "Welcome {{name}}",
        body: "Hi {{name}} at {{company}}. Domains: {{domains}}.",
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/email/messages": { messages: [] },
  "/api/v1/voice/link": {
    googleVoiceUrl:
      "https://accounts.google.com/AccountChooser?Email=shared.voice%40gmail.com&continue=https%3A%2F%2Fvoice.google.com%2Fmessages%3Fauthuser%3Dshared.voice%2540gmail.com",
    googleAccount: "shared.voice@gmail.com",
  },
  "/api/v1/notifications": { notifications: [] },
  "/api/v1/transactions": {
    transactions: [
      {
        id: "transaction-1",
        externalId: "row-1",
        date: "2026-09-03",
        description: "Ada deposit",
        merchant: "River Labs",
        amountCents: 25000,
        source: "tiller",
        accountId: account.id,
        contactId: contact.id,
        matchStatus: "review",
        createdAt: "2026-09-03T12:00:00Z",
        updatedAt: "2026-09-03T12:00:00Z",
      },
    ],
  },
  "/api/v1/attachments": { attachments: [] },
};

function mockAPI(authenticated = true) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "https://kosmos.test");
      const method = (init?.method ?? "GET").toUpperCase();
      const body =
        typeof init?.body === "string"
          ? (JSON.parse(init.body) as Record<string, unknown>)
          : {};
      if (url.pathname === "/api/v1/me")
        return Promise.resolve(
          authenticated ? json(user) : new Response(null, { status: 401 }),
        );
      if (url.pathname === "/auth/logout")
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url.pathname === "/api/v1/accounts" && method === "POST")
        return Promise.resolve(
          json(
            {
              account: {
                ...account,
                id: "account-2",
                name: body.name,
                websites: body.websites,
              },
              contact: {
                ...contact,
                id: "contact-2",
                accountId: "account-2",
                ...(body.primaryContact as object),
              },
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/accounts/account-2" && method === "PATCH")
        return Promise.resolve(
          json({
            ...account,
            id: "account-2",
            name: "Compiler Co",
            websites: [
              { url: "https://compiler.example" },
              { url: "https://compiler.dev" },
            ],
            status: body.status,
          }),
        );
      if (url.pathname === "/api/v1/accounts/account-1" && method === "PATCH")
        return Promise.resolve(json({ ...account, ...body }));
      if (url.pathname === "/api/v1/accounts/account-2")
        return Promise.resolve(
          json({
            account: {
              ...account,
              id: "account-2",
              name: "Compiler Co",
              websites: [
                { url: "https://compiler.example" },
                { url: "https://compiler.dev" },
              ],
            },
            contacts: [],
            opportunities: [],
            documents: [],
          }),
        );
      if (url.pathname === "/api/v1/contacts" && method === "POST")
        return Promise.resolve(
          json(
            {
              ...contact,
              id: "contact-2",
              name: body.name,
              accountId: body.accountId,
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/contacts/contact-1" && method === "PATCH")
        return Promise.resolve(json({ ...contact, ...body }));
      if (
        url.pathname === "/api/v1/contacts/contact-1" &&
        method === "DELETE"
      )
        return Promise.resolve(new Response(null, { status: 204 }));
      if (
        url.pathname === "/api/v1/members/member-1/send-as" &&
        method === "PUT"
      )
        return Promise.resolve(
          json({
            id: "member-1",
            memberId: "member-1",
            memberEmail: user.email,
            email: body.email,
            updatedBy: user.email,
            createdAt: "2026-09-03T12:00:00Z",
            updatedAt: "2026-09-03T12:00:00Z",
          }),
        );
      if (url.pathname === "/api/v1/activities" && method === "POST")
        return Promise.resolve(
          json(
            {
              id: "activity-2",
              contactId: body.contactId,
              opportunityId: "",
              kind: body.kind,
              body: body.body,
              occurredAt: "2026-09-03T13:00:00Z",
              createdAt: "2026-09-03T13:00:00Z",
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/reminders" && method === "POST")
        return Promise.resolve(
          json(
            {
              id: "reminder-2",
              contactId: body.contactId,
              title: body.title,
              dueAt: body.dueAt,
              completed: false,
              createdAt: "2026-09-03T13:00:00Z",
              updatedAt: "2026-09-03T13:00:00Z",
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/reminders/reminder-1" && method === "PATCH")
        return Promise.resolve(
          json({
            ...((responses["/api/v1/reminders"] as { reminders: unknown[] })
              .reminders[0] as object),
            completed: true,
          }),
        );
      if (url.pathname === "/api/v1/opportunities" && method === "POST")
        return Promise.resolve(
          json(
            {
              id: "opportunity-2",
              name: body.name,
              accountId: body.accountId,
              contactId: body.contactId,
              amountCents: body.amountCents,
              stage: body.stage,
              nextStep: body.nextStep,
              closeDate: body.closeDate,
              createdAt: "2026-09-03T13:00:00Z",
              updatedAt: "2026-09-03T13:00:00Z",
            },
            201,
          ),
        );
      if (
        url.pathname === "/api/v1/opportunities/opportunity-1" &&
        method === "PATCH"
      )
        return Promise.resolve(
          json({
            ...(
              responses["/api/v1/opportunities"] as { opportunities: object[] }
            ).opportunities[0],
            ...body,
            updatedAt: "2026-09-04T13:00:00Z",
          }),
        );
      if (
        url.pathname === "/api/v1/opportunities/opportunity-1" &&
        method === "DELETE"
      )
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url.pathname === "/api/v1/documents" && method === "POST")
        return Promise.resolve(
          json(
            {
              id: "document-2",
              title: body.title,
              body: body.body,
              links: body.links,
              createdAt: "2026-09-03T13:00:00Z",
              updatedAt: "2026-09-03T13:00:00Z",
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/documents/document-1" && method === "PATCH")
        return Promise.resolve(
          json({
            ...(responses["/api/v1/documents"] as { documents: object[] })
              .documents[0],
            ...body,
            revision: 2,
            updatedAt: "2026-09-04T13:00:00Z",
          }),
        );
      if (
        url.pathname === "/api/v1/documents/document-1" &&
        method === "DELETE"
      )
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url.pathname === "/api/v1/costs" && method === "POST")
        return Promise.resolve(
          json(
            {
              id: "cost-2",
              vendor: body.vendor,
              description: body.description,
              amountCents: body.amountCents,
              category: body.category,
              incurredOn: body.incurredOn,
              recurring: body.recurring,
              recurrence: body.recurrence,
              taxDeductible: body.taxDeductible,
              notes: body.notes,
              createdAt: "2026-09-03T13:00:00Z",
              updatedAt: "2026-09-03T13:00:00Z",
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/landing/buttons" && method === "POST")
        return Promise.resolve(
          json(
            {
              id: "reports",
              label: "Fishing reports",
              description: "Open reports.",
              href: "https://example.com/reports",
              icon: "globe",
            },
            201,
          ),
        );
      if (url.pathname === "/api/v1/landing/buttons/docs" && method === "PATCH")
        return Promise.resolve(
          json({
            ...(responses["/api/v1/landing"] as { buttons: object[] })
              .buttons[0],
            ...body,
          }),
        );
      if (
        url.pathname === "/api/v1/landing/buttons/docs" &&
        method === "DELETE"
      )
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url.pathname === "/api/v1/contact-sources" && method === "POST")
        return Promise.resolve(
          json(
            { id: "new-source", name: body.name, createdAt: "", updatedAt: "" },
            201,
          ),
        );
      if (url.pathname === "/api/v1/email/send" && method === "POST")
        return Promise.resolve(json({ id: "message-1", status: "sent" }, 201));
      if (url.pathname === "/api/v1/email/sync" && method === "POST")
        return Promise.resolve(
          json({ id: "gmail-job-1", status: "accepted" }, 202),
        );
      if (url.pathname === "/api/v1/email/templates" && method === "POST")
        return Promise.resolve(json({ id: "template-1", ...body }, 201));
      if (
        url.pathname === "/api/v1/integrations/tiller/sync" &&
        method === "POST"
      )
        return Promise.resolve(
          json({ id: "tiller-job-1", status: "accepted" }, 202),
        );
      if (
        url.pathname === "/api/v1/integrations/google-contacts/sync" &&
        method === "POST"
      )
        return Promise.resolve(json({ status: "accepted", queued: 1 }, 202));
      if (
        url.pathname === "/api/v1/integrations/google-contacts" &&
        method === "DELETE"
      )
        return Promise.resolve(new Response(null, { status: 204 }));
      if (url.pathname === "/api/v1/integrations/tiller" && method === "PUT")
        return Promise.resolve(
          json(
            (
              responses["/api/v1/integrations/google"] as {
                connection: unknown;
              }
            ).connection,
          ),
        );
      if (
        url.pathname === "/api/v1/integrations/cloudflare" &&
        method === "PUT"
      )
        return Promise.resolve(
          json({ connected: true, accountId: body.accountId, domainCount: 2 }),
        );
      if (
        url.pathname === "/api/v1/integrations/tiller/webhook" &&
        method === "PUT"
      )
        return Promise.resolve(
          json({ connected: true, endpoint: "/api/v1/webhooks/tiller" }),
        );
      if (
        url.pathname.startsWith(
          "/api/v1/integrations/tiller/product-mappings/",
        ) &&
        method === "PUT"
      )
        return Promise.resolve(
          json({
            id: "mapping-1",
            productId: decodeURIComponent(url.pathname.split("/").pop()!),
            productName: body.productName,
            accountId: body.accountId,
            createdBy: user.email,
            createdAt: "2026-09-03T12:00:00Z",
            updatedAt: "2026-09-03T12:00:00Z",
          }),
        );
      if (
        url.pathname === "/api/v1/transactions/transaction-1" &&
        method === "PATCH"
      )
        return Promise.resolve(
          json({
            ...((
              responses["/api/v1/transactions"] as { transactions: unknown[] }
            ).transactions[0] as object),
            matchStatus: body.matchStatus,
          }),
        );
      if (url.pathname === "/api/v1/attachments" && method === "POST") {
        const form = init?.body as FormData;
        const file = form.get("file") as File;
        return Promise.resolve(
          json(
            {
              id: "attachment-1",
              fileName:
                form.get("kind") === "photo"
                  ? "profile.png"
                  : form.get("recordType") === "document"
                    ? "guide.pdf"
                    : "license.pdf",
              contentType: file?.type || "application/pdf",
              size: file?.size || 512,
              kind: form.get("kind"),
              recordType: form.get("recordType"),
              recordId: form.get("recordId"),
              createdBy: user.email,
              createdAt: "2026-09-03T12:00:00Z",
              downloadUrl: "/download",
              viewUrl:
                form.get("kind") === "photo"
                  ? "/profile.png"
                  : "/download?disposition=inline",
            },
            201,
          ),
        );
      }
      if (
        url.pathname.startsWith("/api/v1/attachments/") &&
        method === "DELETE"
      )
        return Promise.resolve(new Response(null, { status: 204 }));
      const key = url.pathname + url.search;
      return Promise.resolve(
        json(responses[key] ?? responses[url.pathname] ?? {}),
      );
    }),
  );
}

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("Kosmos application", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/");
    vi.stubGlobal("scrollTo", vi.fn());
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows no private workspace information before login", async () => {
    mockAPI(false);
    render(<App />);

    expect(
      await screen.findByRole("link", { name: /continue with google/i }),
    ).toHaveAttribute("href", "/auth/login");
    expect(
      screen.getByText(/approved company google accounts/i),
    ).toBeInTheDocument();
    expect(screen.queryByText("$1,250.00")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("navigation", { name: /workspace/i }),
    ).not.toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["desktop", 1440],
    ["tablet", 768],
    ["mobile", 390],
  ])(
    "keeps every core workspace workflow usable on %s",
    async (_name, width) => {
      window.innerWidth = width;
      mockAPI();
      render(<App />);

      expect(
        await screen.findByRole("heading", {
          name: /good (morning|afternoon|evening), joey/i,
        }),
      ).toBeInTheDocument();
      for (const destination of [
        "Overview",
        "Contacts",
        "Accounts",
        "Opportunities",
        "Documents",
        "Inbox",
        "Operations",
        "Settings",
      ]) {
        expect(
          screen.getByRole("link", { name: destination }),
        ).toBeInTheDocument();
      }
      expect(
        screen.queryByRole("link", { name: "Costs" }),
      ).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole("link", { name: "Contacts" }));
      expect(
        await screen.findByRole("heading", { name: "Contacts" }),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /add contact/i }));
      fireEvent.change(screen.getByLabelText(/full name/i), {
        target: { value: "Grace Hopper" },
      });
      fireEvent.change(screen.getByLabelText(/^account$/i), {
        target: { value: account.id },
      });
      expect(screen.queryByLabelText(/company/i)).not.toBeInTheDocument();
      expect(
        screen.queryByLabelText(/relationship status/i),
      ).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /save contact/i }));
      expect(
        await screen.findByRole("heading", { name: "Grace Hopper" }),
      ).toBeInTheDocument();
      fireEvent.change(
        screen.getByRole("textbox", { name: /activity note/i }),
        { target: { value: "Confirmed the project scope." } },
      );
      fireEvent.click(screen.getByRole("button", { name: /add to timeline/i }));
      expect(
        await screen.findByText("Confirmed the project scope."),
      ).toBeInTheDocument();
      fireEvent.change(
        screen.getByRole("textbox", { name: /what needs to happen/i }),
        { target: { value: "Send the estimate" } },
      );
      fireEvent.change(screen.getByLabelText(/when/i), {
        target: { value: "2026-09-10T10:00" },
      });
      fireEvent.click(screen.getByRole("button", { name: /add reminder/i }));
      expect(await screen.findByText("Send the estimate")).toBeInTheDocument();

      const contactRequest = vi
        .mocked(fetch)
        .mock.calls.find(
          ([input, init]) =>
            String(input) === "/api/v1/contacts" && init?.method === "POST",
        );
      expect(
        new Headers(contactRequest?.[1]?.headers).get("X-Kosmos-CSRF"),
      ).toBe("1");

      fireEvent.click(screen.getByRole("link", { name: "Opportunities" }));
      expect(
        await screen.findByRole("heading", { name: "Opportunities" }),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /add opportunity/i }));
      fireEvent.change(
        screen.getByRole("textbox", { name: /opportunity name/i }),
        { target: { value: "River cleanup" } },
      );
      fireEvent.change(screen.getByRole("combobox", { name: /^account$/i }), {
        target: { value: account.id },
      });
      fireEvent.change(screen.getByRole("spinbutton", { name: /value/i }), {
        target: { value: "2500" },
      });
      fireEvent.click(
        screen.getByRole("button", { name: /save opportunity/i }),
      );
      expect(
        await screen.findByRole("heading", { name: "River cleanup" }),
      ).toBeInTheDocument();

      fireEvent.click(screen.getByRole("link", { name: "Documents" }));
      expect(
        await screen.findByRole("heading", { name: "Documents" }),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /new document/i }));
      fireEvent.change(screen.getByRole("textbox", { name: /^title$/i }), {
        target: { value: "Fishing checklist" },
      });
      fireEvent.change(
        screen.getByRole("textbox", { name: /start writing in markdown/i }),
        { target: { value: "# Before launch" } },
      );
      fireEvent.change(screen.getByLabelText(/link to/i), {
        target: { value: "contact" },
      });
      fireEvent.change(screen.getByLabelText(/linked record/i), {
        target: { value: contact.id },
      });
      fireEvent.click(screen.getByRole("button", { name: /create document/i }));
      expect(
        await screen.findByRole("heading", { name: "Before launch" }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("link", { name: /contact · ada angler/i }),
      ).toHaveAttribute("href", `/contacts/${contact.id}`);

      fireEvent.click(screen.getByRole("link", { name: "Operations" }));
      expect(
        await screen.findByRole("heading", { name: "Business operations" }),
      ).toBeInTheDocument();
      expect(
        await screen.findByRole("heading", { name: "Business costs" }),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /record a cost/i }));
      const now = new Date();
      const localDate = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
      expect(screen.getByLabelText(/^date$/i)).toHaveValue(localDate);
      fireEvent.change(screen.getByRole("textbox", { name: /description/i }), {
        target: { value: "Domain renewal" },
      });
      fireEvent.change(screen.getByRole("spinbutton", { name: /amount/i }), {
        target: { value: "24" },
      });
      fireEvent.click(screen.getByRole("button", { name: /save cost/i }));
      expect(await screen.findByText("Domain renewal")).toBeInTheDocument();

      fireEvent.click(
        screen.getByRole("button", { name: /notifications and follow-ups/i }),
      );
      expect(
        await screen.findByRole("heading", { name: "Activity and follow-ups" }),
      ).toBeInTheDocument();
      fireEvent.click(
        screen.getByRole("button", { name: /complete send proposal/i }),
      );
      await waitFor(() =>
        expect(
          screen.queryByRole("button", { name: /complete send proposal/i }),
        ).not.toBeInTheDocument(),
      );
      fireEvent.click(screen.getByRole("link", { name: "Settings" }));
      expect(
        await screen.findByRole("heading", { name: "Settings" }),
      ).toBeInTheDocument();
      expect(
        await screen.findByRole("heading", { name: /members and roles/i }),
      ).toBeInTheDocument();

      fireEvent.click(screen.getByRole("link", { name: "Inbox" }));
      expect(
        await screen.findByRole("heading", { name: "Communications" }),
      ).toBeInTheDocument();
      fireEvent.change(screen.getByRole("combobox", { name: /^to$/i }), {
        target: { value: "ada@example.com" },
      });
      fireEvent.change(screen.getByRole("textbox", { name: /^subject$/i }), {
        target: { value: "River update" },
      });
      fireEvent.change(screen.getByRole("textbox", { name: /^message$/i }), {
        target: { value: "The plan is ready." },
      });
      fireEvent.click(screen.getByRole("button", { name: /send with gmail/i }));
      expect(
        await screen.findByText(/email sent through your google account/i),
      ).toBeInTheDocument();
      fireEvent.click(
        screen.getByRole("button", { name: /check for replies/i }),
      );
      expect(
        await screen.findByText(/gmail check queued/i),
      ).toBeInTheDocument();

      fireEvent.click(screen.getByRole("link", { name: "Operations" }));
      expect(
        await screen.findByRole("heading", { name: "Business operations" }),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /sync now/i }));
      expect(
        await screen.findByText(/tiller import queued/i),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("link", { name: /export contacts/i }),
      ).toHaveAttribute("href", "/api/v1/exports/contacts");
      fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
      await waitFor(() =>
        expect(
          vi
            .mocked(fetch)
            .mock.calls.some(
              ([input]) =>
                String(input) === "/api/v1/transactions/transaction-1",
            ),
        ).toBe(true),
      );
    },
  );

  it("searches the private workspace and opens a result", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    const search = screen.getByRole("textbox", { name: /search kosmos/i });
    fireEvent.change(search, { target: { value: "river" } });
    fireEvent.submit(search.closest("form")!);
    expect(
      await screen.findByRole("heading", { name: /results for “river”/i }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /ada angler/i }));
    expect(
      await screen.findByRole("heading", { name: "Ada Angler" }),
    ).toBeInTheDocument();
    expect(window.location.pathname).toBe(`/contacts/${contact.id}`);
  });

  it("creates a landing-zone shortcut with CSRF protection", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("button", { name: /add a shortcut/i }));
    fireEvent.change(screen.getByLabelText(/button name/i), {
      target: { value: "Fishing reports" },
    });
    fireEvent.change(screen.getByLabelText(/^link$/i), {
      target: { value: "https://example.com/reports" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save shortcut/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input]) => String(input) === "/api/v1/landing/buttons",
          ),
      ).toBe(true),
    );
    const request = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input]) => String(input) === "/api/v1/landing/buttons",
      );
    expect(new Headers(request?.[1]?.headers).get("X-Kosmos-CSRF")).toBe("1");
  });

  it("lets an administrator edit and confirm deletion of a shortcut", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("button", { name: /edit field notes/i }));
    fireEvent.change(screen.getByLabelText(/button name/i), {
      target: { value: "Team handbook" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/landing/buttons/docs" &&
              init?.method === "PATCH",
          ),
      ).toBe(true),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: /delete field notes/i }),
    );
    fireEvent.click(screen.getByRole("button", { name: /^delete shortcut$/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/landing/buttons/docs" &&
              init?.method === "DELETE",
          ),
      ).toBe(true),
    );
  });

  it("creates an account with multiple websites and its first contact", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Accounts" }));
    expect(
      await screen.findByRole("heading", { name: "Accounts" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /add account/i }));
    fireEvent.change(screen.getByLabelText(/business name/i), {
      target: { value: "Compiler Co" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: /^website 1$/i }), {
      target: { value: "compiler.example" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /add another website/i }),
    );
    fireEvent.change(screen.getByRole("textbox", { name: "Website 2" }), {
      target: { value: "compiler.dev" },
    });
    fireEvent.change(screen.getByLabelText(/full name/i), {
      target: { value: "Grace Hopper" },
    });
    fireEvent.change(screen.getByLabelText(/^email$/i), {
      target: { value: "grace@compiler.example" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save account/i }));

    expect(
      await screen.findByRole("heading", { name: "Compiler Co" }),
    ).toBeInTheDocument();
    const request = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/v1/accounts" && init?.method === "POST",
      );
    const payload = JSON.parse(String(request?.[1]?.body)) as {
      websites: Array<{ url: string }>;
      primaryContact: { name: string; email: string };
    };
    expect(payload.websites).toEqual([
      { url: "compiler.example" },
      { url: "compiler.dev" },
    ]);
    expect(payload.primaryContact).toMatchObject({
      name: "Grace Hopper",
      email: "grace@compiler.example",
    });
  });

  it("opens and edits a contact LinkedIn profile", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Contacts" }));
    fireEvent.click(await screen.findByRole("button", { name: /ada angler/i }));
    expect(
      await screen.findByRole("link", { name: /linkedin/i }),
    ).toHaveAttribute("href", contact.linkedinUrl);
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));
    fireEvent.change(screen.getByLabelText(/linkedin profile/i), {
      target: { value: "https://www.linkedin.com/in/ada-updated" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/contacts/contact-1" &&
              init?.method === "PATCH",
          ),
      ).toBe(true),
    );
    const request = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/v1/contacts/contact-1" &&
          init?.method === "PATCH",
      );
    expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
      linkedinUrl: "https://www.linkedin.com/in/ada-updated",
    });
  });

  it("uploads and displays a private contact photo", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Contacts" }));
    fireEvent.click(await screen.findByRole("button", { name: /ada angler/i }));
    fireEvent.change(await screen.findByLabelText(/upload ada angler photo/i), {
      target: {
        files: [new File(["photo"], "profile.png", { type: "image/png" })],
      },
    });
    expect(await screen.findByAltText(/ada angler profile/i)).toHaveAttribute(
      "src",
      "/profile.png",
    );
  });

  it("edits an existing account and its domains", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Accounts" }));
    fireEvent.click(await screen.findByRole("button", { name: /river labs/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: /edit account/i }),
    );
    fireEvent.change(screen.getByRole("textbox", { name: /^website 1$/i }), {
      target: { value: "https://new.river.example" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /add another website/i }),
    );
    fireEvent.change(screen.getByRole("textbox", { name: /^website 2$/i }), {
      target: { value: "shop.river.example" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => {
      const request = vi
        .mocked(fetch)
        .mock.calls.find(
          ([input, init]) =>
            String(input) === "/api/v1/accounts/account-1" &&
            init?.method === "PATCH",
        );
      expect(JSON.parse(String(request?.[1]?.body)).websites).toEqual([
        { url: "https://new.river.example" },
        { url: "shop.river.example" },
      ]);
    });
  });

  it("removes an existing domain from an account", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Accounts" }));
    fireEvent.click(await screen.findByRole("button", { name: /river labs/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: /edit account/i }),
    );
    fireEvent.click(screen.getByRole("button", { name: /remove website 1/i }));
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => {
      const request = vi
        .mocked(fetch)
        .mock.calls.find(
          ([input, init]) =>
            String(input) === "/api/v1/accounts/account-1" &&
            init?.method === "PATCH",
        );
      expect(JSON.parse(String(request?.[1]?.body)).websites).toEqual([]);
    });
  });

  it("captures a mobile event lead with a new organization source", async () => {
    window.innerWidth = 390;
    window.history.replaceState({}, "", "/lead");
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", { name: /capture the conversation/i });
    fireEvent.change(screen.getByLabelText(/their name/i), {
      target: { value: "Lin Fisher" },
    });
    fireEvent.change(screen.getByLabelText(/contact source/i), {
      target: { value: "__new__" },
    });
    fireEvent.change(screen.getByLabelText(/new contact source/i), {
      target: { value: "Fly fishing expo" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save lead/i }));
    expect(
      await screen.findByText(/lin fisher is in kosmos/i),
    ).toBeInTheDocument();
    const request = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/v1/contacts" && init?.method === "POST",
      );
    expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
      name: "Lin Fisher",
      source: "Fly fishing expo",
    });
  });

  it("confirms opportunity and document deletion", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Opportunities" }));
    fireEvent.click(
      await screen.findByRole("button", { name: /delete website refresh/i }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: /^delete opportunity$/i }),
    );
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/opportunities/opportunity-1" &&
              init?.method === "DELETE",
          ),
      ).toBe(true),
    );
    fireEvent.click(screen.getByRole("link", { name: "Documents" }));
    fireEvent.click(
      await screen.findByRole("button", { name: /delete client kickoff/i }),
    );
    fireEvent.click(screen.getByRole("button", { name: /^delete document$/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/documents/document-1" &&
              init?.method === "DELETE",
          ),
      ).toBe(true),
    );
  });

  it("uploads document files and embeds PDFs by filename", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Documents" }));
    const file = new File(["pdf"], "guide.pdf", { type: "application/pdf" });
    const documentFile = await screen.findByLabelText(/attach a file/i);
    fireEvent.change(documentFile, {
      target: { files: [file] },
    });
    fireEvent.submit(documentFile.closest("form")!);
    expect(await screen.findByText("guide.pdf")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    fireEvent.change(screen.getByLabelText(/start writing in markdown/i), {
      target: { value: "[[guide.pdf]]" },
    });
    expect(screen.getByLabelText(/start writing in markdown/i)).toHaveValue(
      "[[guide.pdf]]",
    );
    fireEvent.click(screen.getByRole("button", { name: /save document/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/documents/document-1" &&
              init?.method === "PATCH" &&
              JSON.parse(String(init.body)).body === "[[guide.pdf]]",
          ),
      ).toBe(true),
    );
    expect(await screen.findByTitle("guide.pdf")).toHaveAttribute(
      "src",
      "/download?disposition=inline",
    );
  });

  it("uploads a receipt while recording a cost", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Operations" }));
    fireEvent.click(
      await screen.findByRole("button", { name: /record a cost/i }),
    );
    fireEvent.change(screen.getByLabelText(/description/i), {
      target: { value: "License" },
    });
    fireEvent.change(screen.getByLabelText(/amount/i), {
      target: { value: "42" },
    });
    const receipt = screen.getByLabelText(/receipt/i);
    fireEvent.change(receipt, {
      target: {
        files: [
          new File(["receipt"], "license.pdf", { type: "application/pdf" }),
        ],
      },
    });
    fireEvent.submit(receipt.closest("form")!);
    expect(
      await screen.findByRole("link", { name: /license.pdf/i }),
    ).toHaveAttribute("href", "/download");
  });

  it("uses the Google profile picture and detects Kosmos Companion", async () => {
    const postMessage = vi.spyOn(window, "postMessage");
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    expect(
      document.querySelector(
        '.avatar img[src="https://images.example/joey.jpg"]',
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Contacts" }));
    fireEvent.click(await screen.findByRole("button", { name: /ada angler/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: /google voice/i }),
    );
    expect(
      await screen.findByText(/install kosmos companion/i),
    ).toBeInTheDocument();
    fireEvent(window, new CustomEvent("kosmos-companion-ready"));
    fireEvent.click(screen.getByRole("button", { name: /google voice/i }));
    await waitFor(() =>
      expect(postMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "KOSMOS_VOICE_PREPARE",
          phone: contact.phone,
          launchUrl: expect.stringContaining("accounts.google.com"),
        }),
        window.location.origin,
      ),
    );
  });

  it("keeps template variables visible and fills them for a known contact", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Inbox" }));
    fireEvent.click(
      await screen.findByRole("button", { name: /welcome note/i }),
    );

    const preview = screen.getByRole("region", { name: /email preview/i });
    expect(within(preview).getByText("Welcome {{name}}"))
      .toBeInTheDocument();
    expect(within(preview).getByText(/Hi {{name}} at {{company}}/))
      .toBeInTheDocument();

    fireEvent.change(screen.getByRole("combobox", { name: /^to$/i }), {
      target: { value: contact.email },
    });
    expect(within(preview).getByText("Welcome Ada Angler"))
      .toBeInTheDocument();
    expect(
      within(preview).getByText(
        "Hi Ada Angler at River Labs. Domains: river.example.",
      ),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /send with gmail/i }));
    await waitFor(() => {
      const request = vi
        .mocked(fetch)
        .mock.calls.find(
          ([input, init]) =>
            String(input) === "/api/v1/email/send" && init?.method === "POST",
        );
      expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
        to: contact.email,
        subject: "Welcome Ada Angler",
        body: "Hi Ada Angler at River Labs. Domains: river.example.",
      });
    });
  });

  it("creates an account document with the account link", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Accounts" }));
    fireEvent.click(await screen.findByRole("button", { name: /river labs/i }));
    fireEvent.click(
      await screen.findByRole("button", { name: /new document/i }),
    );
    fireEvent.change(screen.getByLabelText(/^title$/i), {
      target: { value: "Renewal notes" },
    });
    fireEvent.change(screen.getByLabelText(/start writing/i), {
      target: { value: "# Next steps" },
    });
    fireEvent.click(screen.getByRole("button", { name: /create document/i }));
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(
            ([input, init]) =>
              String(input) === "/api/v1/documents" && init?.method === "POST",
          ),
      ).toBe(true),
    );
    const request = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/v1/documents" && init?.method === "POST",
      );
    expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
      title: "Renewal notes",
      links: [{ type: "account", id: account.id }],
    });
  });

  it("keeps inbox fields usable and explains template variables", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Inbox" }));
    expect(
      await screen.findByRole("heading", { name: /send one good email/i }),
    ).toBeInTheDocument();
    fireEvent.change(
      screen.getByRole("combobox", { name: /google voice contact/i }),
      { target: { value: contact.id } },
    );
    expect(screen.getByRole("link", { name: "Call" })).toHaveAttribute(
      "href",
      `tel:${contact.phone}`,
    );
    expect(screen.getByRole("link", { name: "Text" })).toHaveAttribute(
      "href",
      `sms:${contact.phone}`,
    );
    fireEvent.click(screen.getByRole("button", { name: /template/i }));
    expect(
      screen.getByLabelText(/available template variables/i),
    ).toHaveTextContent("{{name}}");
    expect(
      screen.getByLabelText(/available template variables/i),
    ).toHaveTextContent("{{company}}");
  });

  it("maps verified Gmail aliases and Tiller products from Settings", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Settings" }));
    const sender = await screen.findByLabelText(/send from for joey stout/i);
    fireEvent.change(sender, { target: { value: "hello@nerdswhofish.com" } });
    fireEvent.click(screen.getByRole("button", { name: /verify and save/i }));
    await waitFor(() =>
      expect(
        screen.getByText(/will send kosmos email from hello@nerdswhofish.com/i),
      ).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByLabelText(/tiller signing secret/i), {
      target: { value: `whsec_${"a".repeat(64)}` },
    });
    fireEvent.click(screen.getByRole("button", { name: /connect webhook/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: /purchase webhook connected/i }),
      ).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByLabelText(/tiller product id/i), {
      target: { value: "prod_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
    });
    fireEvent.change(screen.getByLabelText(/product name/i), {
      target: { value: "Guided trip" },
    });
    fireEvent.change(screen.getByLabelText(/kosmos account/i), {
      target: { value: account.id },
    });
    fireEvent.click(screen.getByRole("button", { name: /save mapping/i }));
    expect(
      await screen.findByText(
        /prod_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa · river labs/i,
      ),
    ).toBeInTheDocument();
  });

  it.each([
    ["desktop", 1440],
    ["mobile", 390],
  ])("manages the shared Google Voice contacts account on %s", async (_name, width) => {
    window.innerWidth = width;
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Settings" }));
    expect(
      await screen.findByRole("heading", { name: /shared contacts connected/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("shared.voice@gmail.com")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /sync now/i }));
    expect(
      await screen.findByText(/1 kosmos contact queued/i),
    ).toBeInTheDocument();
    expect(
      vi.mocked(fetch).mock.calls.some(
        ([input, init]) =>
          String(input) === "/api/v1/integrations/google-contacts/sync" &&
          init?.method === "POST",
      ),
    ).toBe(true);
  });

  it.each([
    ["desktop", 1440],
    ["mobile", 390],
  ])("deletes a contact on %s", async (_name, width) => {
    window.innerWidth = width;
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Contacts" }));
    fireEvent.click(await screen.findByRole("button", { name: /ada angler/i }));
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    expect(
      await screen.findByRole("heading", { name: /delete this contact/i }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /delete contact/i }));
    expect(
      await screen.findByRole("heading", { name: "Contacts" }),
    ).toBeInTheDocument();
    expect(
      vi.mocked(fetch).mock.calls.some(
        ([input, init]) =>
          String(input) === "/api/v1/contacts/contact-1" &&
          init?.method === "DELETE",
      ),
    ).toBe(true);
  });

  it.each([
    ["desktop", 1440],
    ["tablet", 768],
    ["mobile", 390],
  ])(
    "moves opportunities and opens their account on %s",
    async (_name, width) => {
      window.innerWidth = width;
      mockAPI();
      render(<App />);
      await screen.findByRole("heading", {
        name: /good (morning|afternoon|evening)/i,
      });
      fireEvent.click(screen.getByRole("link", { name: "Opportunities" }));
      const stage = await screen.findByLabelText(/stage for website refresh/i);
      fireEvent.change(stage, { target: { value: "won" } });
      await waitFor(() =>
        expect(
          vi
            .mocked(fetch)
            .mock.calls.some(
              ([input, init]) =>
                String(input) === "/api/v1/opportunities/opportunity-1" &&
                init?.method === "PATCH",
            ),
        ).toBe(true),
      );
      fireEvent.click(screen.getByRole("tab", { name: "won" }));
      const opportunity = await screen.findByRole("link", {
        name: /open website refresh account/i,
      });
      fireEvent.click(opportunity);
      expect(
        await screen.findByRole("heading", { name: "River Labs" }),
      ).toBeInTheDocument();
    },
  );

  it("drags an opportunity between pipeline stages on desktop", async () => {
    window.innerWidth = 1440;
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Opportunities" }));
    const opportunity = await screen.findByRole("link", {
      name: /open website refresh account/i,
    });
    const transfer = {
      effectAllowed: "none",
      setData: vi.fn(),
      getData: vi.fn(() => "opportunity-1"),
    };
    fireEvent.dragStart(opportunity, { dataTransfer: transfer });
    fireEvent.drop(screen.getByRole("region", { name: /new stage/i }), {
      dataTransfer: transfer,
    });
    await waitFor(() => {
      const request = vi
        .mocked(fetch)
        .mock.calls.find(
          ([input, init]) =>
            String(input) === "/api/v1/opportunities/opportunity-1" &&
            init?.method === "PATCH",
        );
      expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
        stage: "new",
      });
    });
  });

  it("connects Cloudflare with a dedicated credential", async () => {
    mockAPI();
    render(<App />);
    await screen.findByRole("heading", {
      name: /good (morning|afternoon|evening)/i,
    });
    fireEvent.click(screen.getByRole("link", { name: "Settings" }));
    expect(
      await screen.findByRole("heading", { name: /connect your domains/i }),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/cloudflare account id/i), {
      target: { value: "0123456789abcdef0123456789abcdef" },
    });
    fireEvent.change(screen.getByLabelText(/dedicated api token/i), {
      target: { value: "dedicated-token-for-kosmos" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /connect cloudflare/i }),
    );
    expect(
      await screen.findByText(/2 domains are ready to link/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /domain inventory connected/i }),
    ).toBeInTheDocument();
  });
});
