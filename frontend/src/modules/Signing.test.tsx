import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../app/App";
import { Signing } from "./Signing";
import { PublicSigning } from "./PublicSigning";
import { FieldOverlay } from "./SigningFields";
import * as signingApi from "./signingApi";
import {
  boundedField,
  consentText,
  signingCredential,
  type SigningField,
  type SigningRequest,
  type SigningSession,
} from "./signingApi";

vi.mock("../components/SigningPDF", () => ({
  SigningPDF: ({
    children,
    onReady,
  }: {
    children?: React.ReactNode;
    onReady?: (ready: boolean) => void;
  }) => {
    useEffect(() => {
      onReady?.(true);
    }, [onReady]);
    return <div aria-label="Document preview">{children}</div>;
  },
}));

const field: SigningField = {
  id: "signature-1",
  type: "signature",
  label: "Your signature",
  page: 1,
  x: 0.1,
  y: 0.5,
  width: 0.4,
  height: 0.05,
  required: true,
};
const draft: SigningRequest = {
  id: "request-1",
  title: "Website agreement",
  fileName: "agreement.pdf",
  status: "draft",
  pages: [
    { width: 612, height: 792 },
    { width: 612, height: 792 },
  ],
  fields: [],
  revision: 1,
  signerName: "Ada Angler",
  signerEmail: "ada@example.com",
  createdAt: "2026-09-04T12:00:00Z",
  updatedAt: "2026-09-04T12:00:00Z",
  originalSHA256: "original-hash",
};
const pending: SigningRequest = {
  ...draft,
  status: "pending",
  fields: [
    field,
    { ...field, id: "text-1", type: "text", label: "Job title", page: 2 },
  ],
};
const token = "a".repeat(64);
const signingSession: SigningSession = {
  ipAddress: "2001:db8::42",
  userAgent: "Mozilla/5.0 Example Browser/1.0",
  capturedAt: "2026-09-05T14:25:30Z",
  source: "direct",
};

beforeEach(() => {
  window.history.replaceState({}, "", `/sign#request-1.${token}`);
  vi.stubGlobal("scrollTo", vi.fn());
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
});

describe("signing setup", () => {
  it.each([
    { session: signingSession, location: "Unknown" },
    { session: { ...signingSession, city: "Richmond", region: "Virginia", country: "US", source: "cloudflare" }, location: "Richmond, Virginia, US" },
  ])("shows completed session evidence with location $location", async ({ session, location }) => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...draft, status: "completed", session })));
    render(<Signing id="request-1" navigate={vi.fn()} />);
    const card = await screen.findByRole("region", { name: "Signing session" });
    expect(within(card).getByText("2001:db8::42")).toBeInTheDocument();
    expect(within(card).getByText(location)).toBeInTheDocument();
    expect(card.querySelector("time")).toHaveAttribute("datetime", signingSession.capturedAt);
    expect(card).toHaveTextContent("Location is approximate and browser details are self-reported.");
    expect(card).toHaveTextContent("This record is not proof of identity.");
  });

  it("renders raw browser details as inert text", async () => {
    const browser = '<img data-attacker="true" src=x onerror="alert(1)">';
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...draft, status: "completed", session: { ...signingSession, userAgent: browser } })));
    const { container } = render(<Signing id="request-1" navigate={vi.fn()} />);
    fireEvent.click(await screen.findByText("Browser-reported details"));
    expect(screen.getByText(browser)).toBeInTheDocument();
    expect(container.querySelector("[data-attacker]")).toBeNull();
  });

  it("keeps older completed records readable without session evidence", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...draft, status: "completed" })));
    render(<Signing id="request-1" navigate={vi.fn()} />);
    expect(await screen.findByRole("heading", { name: "Signed, sealed, saved." })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Signing session" })).not.toBeInTheDocument();
  });

  it("keeps the upload busy during preparation and preserves the draft when preparation fails", async () => {
    let rejectUpload!: (reason: Error) => void;
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") return new Promise<Response>((_resolve, reject) => { rejectUpload = reject; });
      return Response.json({ requests: [] });
    });
    vi.stubGlobal("fetch", fetcher);
    render(<Signing navigate={vi.fn()} />);
    const fileInput = await screen.findByLabelText("PDF document (up to 10 MB, 50 pages)");
    fireEvent.change(fileInput, { target: { files: [new File(["%PDF"], "forms.pdf", { type: "application/pdf" })] } });
    fireEvent.change(screen.getByLabelText("Document title"), { target: { value: "Form agreement" } });
    const form = screen.getByRole("button", { name: "Upload and place fields" }).closest("form")!;
    fireEvent.submit(form);
    expect(screen.getByRole("button", { name: "Preparing PDF…" })).toBeDisabled();
    expect(form).toHaveAttribute("aria-busy", "true");
    expect(fileInput).toBeDisabled();
    expect(screen.getByLabelText("Document title")).toBeDisabled();
    fireEvent.submit(form);
    expect(fetcher.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
    await act(async () => rejectUpload(new Error("Preparation failed. Please try again.")));
    expect(await screen.findByText("Preparation failed. Please try again.")).toBeInTheDocument();
    expect(screen.getByLabelText("Document title")).toHaveValue("Form agreement");
    expect(screen.getByRole("button", { name: "Upload and place fields" })).toBeEnabled();
    expect(fileInput).toBeEnabled();
  });

  it("explains a flattened copy and offers the retained upload only to the sender", async () => {
    const download = vi.spyOn(signingApi, "downloadPDF").mockResolvedValue();
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...draft, flattened: true, uploadedSHA256: "upload-hash" })));
    render(<Signing id="request-1" navigate={vi.fn()} />);
    expect(await screen.findByRole("region", { name: "Prepared PDF" })).toHaveTextContent("Document text is no longer selectable");
    fireEvent.click(screen.getByRole("button", { name: "Download uploaded PDF" }));
    await waitFor(() => expect(download).toHaveBeenCalledWith("/api/v1/signing-requests/request-1/pdf?uploaded=true", "uploaded-agreement.pdf"));
    await waitFor(() => expect(screen.getByRole("button", { name: "Download prepared PDF" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Download prepared PDF" }));
    await waitFor(() => expect(download).toHaveBeenCalledWith("/api/v1/signing-requests/request-1/pdf", "agreement.pdf"));
  });

  it("keeps ordinary PDFs free of preparation notices and duplicate downloads", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(draft)));
    render(<Signing id="request-1" navigate={vi.fn()} />);
    expect(await screen.findByRole("button", { name: "Download original" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Prepared PDF" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Download uploaded PDF" })).not.toBeInTheDocument();
  });

  it("includes older signing requests by following collection pagination", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => Response.json(String(input).includes("cursor=older")
      ? { requests: [{ ...draft, id: "older-request", title: "Earlier agreement" }], page: { limit: 25 } }
      : { requests: [draft], page: { limit: 25, nextCursor: "older" } }));
    vi.stubGlobal("fetch", fetcher);
    render(<Signing navigate={vi.fn()} />);
    expect(await screen.findByRole("button", { name: /Earlier agreement/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Website agreement/ })).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("uploads the PDF using the existing CSRF-aware API", async () => {
    const navigate = vi.fn();
    const fetcher = vi.fn(
      async (_path: RequestInfo | URL, init?: RequestInit) =>
        Response.json(init?.method === "POST" ? draft : { requests: [] }),
    );
    vi.stubGlobal("fetch", fetcher);
    render(<Signing navigate={navigate} />);
    const file = new File(["%PDF-1.7"], "agreement.pdf", {
      type: "application/pdf",
    });
    fireEvent.change(
      await screen.findByLabelText("PDF document (up to 10 MB, 50 pages)"),
      { target: { files: [file] } },
    );
    fireEvent.change(screen.getByLabelText("Document title"), {
      target: { value: "Website agreement" },
    });
    fireEvent.submit(
      screen
        .getByRole("button", { name: "Upload and place fields" })
        .closest("form")!,
    );
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith("/documents/signing/request-1"),
    );
    const init = fetcher.mock.calls.find(
      ([, init]) => init?.method === "POST",
    )![1]!;
    expect((init.body as FormData).get("file")).toBe(file);
    expect((init.body as FormData).get("title")).toBe("Website agreement");
    expect(new Headers(init.headers).get("X-Kosmos-CSRF")).toBe("1");
  });

  it("saves field geometry before issuing a link and makes the issued document immutable", async () => {
    let current = structuredClone(draft);
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "PUT") {
          current = {
            ...current,
            fields: JSON.parse(String(init.body)).fields,
            revision: 2,
          };
          return Response.json(current);
        }
        if (String(input).endsWith("/link")) {
          current = { ...current, status: "pending", revision: 3 };
          return Response.json({
            request: current,
            signingUrl: `/sign#request-1.${token}`,
          });
        }
        return Response.json(current);
      },
    );
    vi.stubGlobal("fetch", fetcher);
    render(<Signing id="request-1" navigate={vi.fn()} />);
    expect(
      await screen.findByRole("button", { name: "Create signing link" }),
    ).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Signature" }));
    fireEvent.change(screen.getByLabelText("Left (%)"), {
      target: { value: "25" },
    });
    fireEvent.change(screen.getByLabelText("Field page"), {
      target: { value: "2" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Create signing link" }),
    );
    expect(await screen.findByLabelText("Private signing link")).toHaveValue(
      `${window.location.origin}/sign#request-1.${token}`,
    );
    expect(screen.queryByLabelText("Left (%)")).not.toBeInTheDocument();
    expect(current.fields[0]).toMatchObject({
      x: 0.25,
      page: 2,
      required: true,
    });
    const linkCall = fetcher.mock.calls.find(([path]) =>
      String(path).endsWith("/link"),
    )!;
    expect(JSON.parse(String(linkCall[1]?.body)).revision).toBe(2);
  });

  it("revokes only after explicit confirmation", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) =>
      Response.json(
        String(input).endsWith("/revoke")
          ? { ...pending, status: "revoked" }
          : pending,
      ),
    );
    vi.stubGlobal("fetch", fetcher);
    render(<Signing id="request-1" navigate={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Revoke link" }));
    expect(fetcher).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Yes, revoke link" }));
    expect(
      await screen.findByText("Signing link revoked."),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Revoke link" }),
    ).not.toBeInTheDocument();
  });

  it("supports keyboard placement and constrains fields to the document", () => {
    const onChange = vi.fn();
    render(
      <FieldOverlay
        field={{ ...field, x: 0.6 }}
        editable
        onChange={onChange}
      />,
    );
    fireEvent.keyDown(
      screen.getByRole("button", { name: /Select Your signature/ }),
      { key: "ArrowRight", shiftKey: true },
    );
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ x: 0.6 }),
    );
    fireEvent.keyDown(
      screen.getByRole("button", { name: "Resize Your signature" }),
      { key: "ArrowDown" },
    );
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ height: 0.055 }),
    );
    expect(
      boundedField({ ...field, x: -1, y: 5, width: 0, height: 0 }),
    ).toMatchObject({ x: 0, y: 0.985, width: 0.05, height: 0.015 });
  });
});

describe("customer signing", () => {
  it("discloses recorded connection details before consent and describes them to screen readers", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(pending)));
    render(<PublicSigning />);
    const checkbox = await screen.findByRole("checkbox", { name: consentText });
    expect(checkbox).toHaveAccessibleDescription("Your IP address, browser details, and approximate location will be recorded with your signature and shared with the sender.");
    expect(checkbox).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Agree & finish signing" })).toBeDisabled();
  });

  it("lets a customer review the prepared document without exposing the raw upload", async () => {
    const download = vi.spyOn(signingApi, "downloadPDF").mockResolvedValue();
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...pending, flattened: true })));
    render(<PublicSigning />);
    expect(await screen.findByText("This document was prepared as a fixed copy for signing. Review each page before you sign.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Download uploaded PDF" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download document to review" }));
    await waitFor(() => expect(download).toHaveBeenCalledWith("/api/v1/signing/request-1/pdf", "agreement.pdf", token));
  });

  it.each([320, 768, 1440])(
    "requires all fields and explicit consent before submitting at width %s",
    async (width) => {
      vi.stubGlobal("innerWidth", width);
      const fetcher = vi.fn(
        async (_input: RequestInfo | URL, init?: RequestInit) =>
          Response.json(
            init?.method === "POST"
              ? { ...pending, status: "completed" }
              : pending,
          ),
      );
      vi.stubGlobal("fetch", fetcher);
      render(<PublicSigning />);
      const button = await screen.findByRole("button", {
        name: "Agree & finish signing",
      });
      expect(button).toBeDisabled();
      fireEvent.change(screen.getByPlaceholderText("Type your signature"), {
        target: { value: "Ada Angler" },
      });
      fireEvent.click(screen.getByLabelText(consentText));
      expect(button).toBeDisabled();
      fireEvent.change(screen.getByPlaceholderText("Enter your answer"), {
        target: { value: "Owner" },
      });
      expect(button).toBeEnabled();
      fireEvent.click(button);
      expect(
        await screen.findByRole("heading", { name: "You’re all signed." }),
      ).toBeInTheDocument();
      const submitted = fetcher.mock.calls.find(
        ([, init]) => init?.method === "POST",
      )!;
      expect(submitted[0]).toBe("/api/v1/signing/request-1/complete");
      expect(
        new Headers(submitted[1]?.headers).get("X-Kosmos-Signing-Token"),
      ).toBe(token);
      expect(new Headers(submitted[1]?.headers).get("X-Kosmos-CSRF")).toBe("1");
      expect(JSON.parse(String(submitted[1]?.body))).toEqual({
        signerName: "Ada Angler",
        values: { "signature-1": "Ada Angler", "text-1": "Owner" },
        consent: true,
      });
      expect(
        fetcher.mock.calls.every(([path]) => !String(path).includes(token)),
      ).toBe(true);
    },
  );

  it("keeps entered values and consent when completion fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path, init) =>
        init?.method === "POST"
          ? Response.json(
              { error: "Temporary storage failure" },
              { status: 503 },
            )
          : Response.json({ ...pending, fields: [field] }),
      ),
    );
    render(<PublicSigning />);
    fireEvent.change(
      await screen.findByPlaceholderText("Type your signature"),
      { target: { value: "Ada Angler" } },
    );
    fireEvent.click(screen.getByLabelText(consentText));
    fireEvent.click(
      screen.getByRole("button", { name: "Agree & finish signing" }),
    );
    expect(
      await screen.findByText("Temporary storage failure"),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Type your signature")).toHaveValue(
      "Ada Angler",
    );
    expect(screen.getByLabelText(consentText)).toBeChecked();
    expect(
      screen.getByRole("button", { name: "Agree & finish signing" }),
    ).toBeEnabled();
  });

  it("does not offer completion on an expired or revoked request", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ ...pending, expiresAt: "2020-01-01T00:00:00Z" }),
      ),
    );
    render(<PublicSigning />);
    expect(
      await screen.findByRole("heading", {
        name: "This signing link has expired.",
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Agree & finish signing" }),
    ).not.toBeInTheDocument();
  });

  it("opens a public link without checking the Kosmos session", async () => {
    const fetcher = vi.fn(async (_path: RequestInfo | URL) =>
      Response.json(pending),
    );
    vi.stubGlobal("fetch", fetcher);
    render(<App />);
    expect(
      await screen.findByRole("heading", { name: "Website agreement" }),
    ).toBeInTheDocument();
    expect(fetcher.mock.calls.map(([path]) => path)).not.toContain(
      "/api/v1/me",
    );
  });

  it("does not make network requests for malformed capability links", () => {
    expect(signingCredential("#request.secret?extra")).toBeNull();
    expect(signingCredential("#request.secret")).toEqual({
      id: "request",
      token: "secret",
    });
    window.history.replaceState({}, "", "/sign");
    const fetcher = vi.fn();
    vi.stubGlobal("fetch", fetcher);
    render(<PublicSigning />);
    expect(
      screen.getByRole("heading", { name: "This signing link is incomplete." }),
    ).toBeInTheDocument();
    expect(fetcher).not.toHaveBeenCalled();
  });
});
