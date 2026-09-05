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
const participants = [
  { id: "customer", name: "Ada Angler", email: "ada@example.com" },
  { id: "staff", name: "Sam Staff", email: "sam@example.com" },
];

beforeEach(() => {
  window.history.replaceState({}, "", `/sign#request-1.${token}`);
  vi.stubGlobal("scrollTo", vi.fn());
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  window.history.replaceState({}, "", "/");
});

describe("signing setup", () => {
  it("assigns fields to each signer, requires every signature, and issues separate links", async () => {
    let current: SigningRequest = { ...draft, signers: participants };
    const fetcher = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") current = { ...current, ...JSON.parse(String(init.body)), revision: 2 };
      if (String(path).endsWith("/link")) {
        current = { ...current, status: "pending", revision: 3 };
        return Response.json({ request: current, signingLinks: participants.map((signer) => ({ signerId: signer.id, signingUrl: `/sign#request-1.${signer.id}token` })) });
      }
      return Response.json(current);
    });
    vi.stubGlobal("fetch", fetcher);
    render(<Signing id="request-1" navigate={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Signature" }));
    expect(screen.getByRole("button", { name: "Create signing links" })).toBeDisabled();
    expect(screen.getByLabelText("Assigned signer")).toHaveValue("customer");
    fireEvent.change(screen.getByLabelText("Place fields for"), { target: { value: "staff" } });
    fireEvent.click(screen.getByRole("button", { name: "Signature" }));
    expect(screen.getByLabelText("Assigned signer")).toHaveValue("staff");
    expect(screen.getByRole("button", { name: "Create signing links" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Create signing links" }));
    expect(await screen.findByLabelText("Signing link for Ada Angler")).toHaveValue(`${window.location.origin}/sign#request-1.customertoken`);
    expect(screen.getByLabelText("Signing link for Sam Staff")).toHaveValue(`${window.location.origin}/sign#request-1.stafftoken`);
    expect(current.fields.map((item) => item.signerId)).toEqual(["customer", "staff"]);
    expect(current.signers).toEqual(participants);
    expect(screen.getByText("0 of 2 signed")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add signer" })).not.toBeInTheDocument();
  });

  it("does not remove a signer while fields are still assigned to them", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...draft, signers: participants, fields: [{ ...field, signerId: "customer" }] })));
    render(<Signing id="request-1" navigate={vi.fn()} />);
    expect(await screen.findByRole("button", { name: "Remove signer 1" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Selected field"), { target: { value: field.id } });
    fireEvent.change(screen.getByLabelText("Assigned signer"), { target: { value: "staff" } });
    fireEvent.click(screen.getByRole("button", { name: "Remove signer 1" }));
    expect(screen.getByLabelText("Signer 1 name")).toHaveValue("Sam Staff");
    expect(screen.getByLabelText("Assigned signer")).toHaveValue("staff");
  });
  it("creates replacement download links using the current revision and selected lifetime", async () => {
    let current: SigningRequest = { ...draft, status: "completed", revision: 5 };
    const fetcher = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        current = { ...current, revision: current.revision + 1, downloadExpiresAt: "2099-09-05T12:00:00Z" };
        return Response.json({ request: current, downloadUrl: `/sign#request-1.download${current.revision}`, expiresAt: current.downloadExpiresAt });
      }
      return Response.json(current);
    });
    vi.stubGlobal("fetch", fetcher);
    render(<Signing id="request-1" navigate={vi.fn()} />);
    const create = await screen.findByRole("button", { name: "Create download link" });
    expect(screen.getByLabelText("Download link expires in")).toHaveValue("60");
    fireEvent.click(create);
    expect(await screen.findByLabelText("Private download link")).toHaveValue(`${window.location.origin}/sign#request-1.download6`);
    fireEvent.change(screen.getByLabelText("Download link expires in"), { target: { value: "1440" } });
    fireEvent.click(create);
    await waitFor(() => expect(screen.getByLabelText("Private download link")).toHaveValue(`${window.location.origin}/sign#request-1.download7`));
    expect(fetcher.mock.calls.filter(([, init]) => init?.method === "POST").map(([, init]) => JSON.parse(String(init?.body)))).toEqual([
      { revision: 5, expiresMinutes: 60 }, { revision: 6, expiresMinutes: 1440 },
    ]);
    expect(screen.getByText(/A new download link replaces/)).toBeInTheDocument();
  });

  it.each(["draft", "completed", "revoked"] as const)("deletes a %s document only after inline confirmation", async (status) => {
    const navigate = vi.fn();
    const fetcher = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => init?.method === "DELETE"
      ? new Response(null, { status: 204 }) : Response.json({ ...draft, status, revision: 4 }));
    vi.stubGlobal("fetch", fetcher);
    render(<Signing id="request-1" navigate={navigate} />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete document" }));
    expect(fetcher).toHaveBeenCalledTimes(1);
    const confirmation = screen.getByRole("group", { name: "Confirm document deletion" });
    expect(confirmation).toHaveTextContent("invalidates all its links");
    expect(confirmation).toHaveTextContent("Stored files are purged when retention allows");
    fireEvent.click(screen.getByRole("button", { name: "Keep document" }));
    expect(screen.queryByRole("group", { name: "Confirm document deletion" })).not.toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Delete document" }));
    fireEvent.click(screen.getByRole("button", { name: "Yes, delete document" }));
    await waitFor(() => expect(navigate).toHaveBeenCalledWith("/documents/signing"));
    const deletion = fetcher.mock.calls.find(([, init]) => init?.method === "DELETE")!;
    expect(JSON.parse(String(deletion[1]?.body))).toEqual({ revision: 4, confirmed: true });
    expect(new Headers(deletion[1]?.headers).get("X-Kosmos-CSRF")).toBe("1");
  });

  it("requires revoking a pending request before showing deletion controls", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(pending)));
    render(<Signing id="request-1" navigate={vi.fn()} />);
    expect(await screen.findByText("Revoke the active signing link before deleting this document.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete document" })).not.toBeInTheDocument();
  });
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
  it("collects only this recipient's fields and shows their signed state before everyone finishes", async () => {
    const request: SigningRequest = { ...pending, signers: participants, currentSignerId: "customer", fields: [{ ...field, signerId: "customer" }, { ...field, id: "staff-signature", label: "Staff signature", signerId: "staff" }] };
    const fetcher = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => Response.json(init?.method === "POST"
      ? { ...request, signers: [{ ...participants[0], signedAt: "2026-09-05T12:00:00Z" }, participants[1]], accessExpiresAt: "2099-09-05T12:15:00Z" }
      : request));
    vi.stubGlobal("fetch", fetcher);
    const download = vi.spyOn(signingApi, "downloadPDF").mockResolvedValue();
    render(<PublicSigning />);
    fireEvent.change(await screen.findByPlaceholderText("Type your signature"), { target: { value: "Ada Angler" } });
    expect(screen.getByLabelText(consentText)).toHaveAccessibleDescription("Your IP address, browser details, and approximate location will be recorded with your signature and shared with the sender and other signers.");
    expect(screen.queryByText(/Staff signature/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByLabelText(consentText));
    fireEvent.click(screen.getByRole("button", { name: "Agree & finish signing" }));
    expect(await screen.findByText("Waiting for Sam Staff to sign.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Agree & finish signing" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download current signed copy" }));
    await waitFor(() => expect(download).toHaveBeenCalledWith("/api/v1/signing/request-1/pdf?completed=true", "signed-agreement.pdf", token));
    const submission = fetcher.mock.calls.find(([, init]) => init?.method === "POST")!;
    expect(JSON.parse(String(submission[1]?.body)).values).toEqual({ "signature-1": "Ada Angler" });
  });

  it("allows an unsigned recipient to sign while another is complete", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...pending, signers: [{ ...participants[0], signedAt: "2026-09-05T12:00:00Z" }, participants[1]], currentSignerId: "staff", fields: [{ ...field, signerId: "staff" }] })));
    render(<PublicSigning />);
    expect(await screen.findByLabelText("Your full name")).toHaveValue("Sam Staff");
    expect(screen.getByRole("button", { name: "Agree & finish signing" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "You’re all signed." })).not.toBeInTheDocument();
  });
  it("uses the token-specific deadline and removes the signed PDF at expiry", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-05T12:00:00Z"));
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...pending, status: "completed", accessExpiresAt: "2026-09-05T12:00:02Z", postSignExpiresAt: "2026-09-05T12:15:00Z" })));
    render(<PublicSigning />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(screen.getByRole("timer")).toHaveTextContent("0:02 remaining");
    expect(screen.getByLabelText("Document preview")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Download signed PDF" })).toBeEnabled();
    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
    expect(screen.getByRole("heading", { name: "This download link has expired." })).toBeInTheDocument();
    expect(screen.queryByLabelText("Document preview")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Download signed PDF" })).not.toBeInTheDocument();
    expect(screen.getByText("Your signature is saved.")).toBeInTheDocument();
  });

  it.each(["focus", "visibilitychange"])("checks elapsed wall time when returning through %s", async (event) => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-05T12:00:00Z"));
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ ...pending, status: "completed", accessExpiresAt: "2026-09-05T12:15:00Z" })));
    render(<PublicSigning />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    vi.setSystemTime(new Date("2026-09-05T12:16:00Z"));
    await act(async () => { (event === "focus" ? window : document).dispatchEvent(new Event(event)); });
    expect(screen.queryByLabelText("Document preview")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "This download link has expired." })).toBeInTheDocument();
  });

  it.each([404, 410])("offers a new-link recovery for inaccessible metadata (%s)", async (status) => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: { code: "signing_unavailable", message: "Unavailable" } }, { status })));
    render(<PublicSigning />);
    expect(await screen.findByRole("heading", { name: "This document link is no longer available." })).toBeInTheDocument();
    expect(screen.getByText(/Ask the sender for a new link/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Document preview")).not.toBeInTheDocument();
  });
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
