import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SigningPDF } from "./SigningPDF";
import { getDocument } from "pdfjs-dist";
import { pdfBytes } from "../modules/signingApi";
import { reportPDFPreviewFailure } from "../telemetry";

vi.mock("pdfjs-dist", () => ({ getDocument: vi.fn(), GlobalWorkerOptions: {} }));
vi.mock("../modules/signingApi", () => ({ pdfBytes: vi.fn() }));
vi.mock("../telemetry", () => ({ reportPDFPreviewFailure: vi.fn() }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function pdfPage(text = "Document text") {
  return {
    getViewport: vi.fn(() => ({ width: 612, height: 792 })),
    render: vi.fn(() => ({ promise: Promise.resolve(), cancel: vi.fn() })),
    getTextContent: vi.fn(() => { throw new TypeError("stream has no async iterator"); }),
    streamTextContent: vi.fn(() => ({ getReader: () => textReader(text) })),
  };
}

type TextChunk = { items: { str: string }[] };
function textReader(text: string) {
  return {
    read: vi.fn<() => Promise<ReadableStreamReadResult<TextChunk>>>()
      .mockResolvedValueOnce({ done: false, value: { items: [{ str: text }] } })
      .mockResolvedValue({ done: true, value: undefined }),
    cancel: vi.fn(async () => undefined),
    releaseLock: vi.fn(),
  };
}

const first = pdfPage("First page");
const second = pdfPage("Second page");
const document = { numPages: 2, getPage: vi.fn() };
const destroy = vi.fn(async () => undefined);
const props = { path: "/api/v1/signing/example/pdf", page: 1, width: 612, height: 792 };

beforeEach(() => {
  vi.mocked(pdfBytes).mockResolvedValue(new ArrayBuffer(8));
  document.getPage.mockImplementation(async (page: number) => page === 1 ? first : second);
  vi.mocked(getDocument).mockReturnValue({ promise: Promise.resolve(document), destroy } as unknown as ReturnType<typeof getDocument>);
  first.render.mockImplementation(() => ({ promise: Promise.resolve(), cancel: vi.fn() }));
  second.render.mockImplementation(() => ({ promise: Promise.resolve(), cancel: vi.fn() }));
  first.streamTextContent.mockImplementation(() => ({ getReader: () => textReader("First page") }));
  second.streamTextContent.mockImplementation(() => ({ getReader: () => textReader("Second page") }));
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe("PDF preview lifecycle", () => {
  it("keeps a successfully rendered PDF usable when optional text extraction fails", async () => {
    const reader = textReader("First page");
    reader.read.mockReset().mockRejectedValue(new Error("text extraction failed"));
    first.streamTextContent.mockReturnValue({ getReader: () => reader });
    const onReady = vi.fn();
    render(<SigningPDF {...props} onReady={onReady}><button>Sign here</button></SigningPDF>);
    expect(await screen.findByRole("button", { name: "Sign here" })).toBeInTheDocument();
    expect(onReady).toHaveBeenLastCalledWith(true);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText(/Page text is unavailable/)).toBeInTheDocument();
    expect(reportPDFPreviewFailure).toHaveBeenCalledWith("text", expect.any(Error));
  });

  it("reports readiness without waiting for slow text extraction", async () => {
    const text = deferred<ReadableStreamReadResult<TextChunk>>();
    const reader = textReader("First page");
    reader.read.mockReset().mockReturnValueOnce(text.promise).mockResolvedValue({ done: true, value: undefined });
    first.streamTextContent.mockReturnValue({ getReader: () => reader });
    render(<SigningPDF {...props}><button>Sign here</button></SigningPDF>);
    expect(await screen.findByRole("button", { name: "Sign here" })).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    await act(async () => text.resolve({ done: false, value: { items: [{ str: "Eventual text" }] } }));
    expect(await screen.findByText("Eventual text")).toBeInTheDocument();
  });

  it("clears a failed page error when the next page renders successfully", async () => {
    first.render.mockImplementation(() => ({ promise: Promise.reject(new Error("canvas failed")), cancel: vi.fn() }));
    const view = render(<SigningPDF {...props} />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/could not/);
    view.rerender(<SigningPDF {...props} page={2}><button>Sign here</button></SigningPDF>);
    expect(await screen.findByRole("button", { name: "Sign here" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("Second page")).toBeInTheDocument();
  });

  it("isolates a new page canvas while cancellation of the previous render settles", async () => {
    const oldRender = deferred<void>();
    const cancel = vi.fn();
    first.render.mockReturnValue({ promise: oldRender.promise, cancel });
    const view = render(<SigningPDF {...props} />);
    await waitFor(() => expect(first.render).toHaveBeenCalled());
    const oldCanvas = screen.getByRole("img", { name: "Document page 1" });
    view.rerender(<SigningPDF {...props} page={2}><button>Sign here</button></SigningPDF>);
    expect(await screen.findByRole("button", { name: "Sign here" })).toBeInTheDocument();
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("img", { name: "Document page 2" })).not.toBe(oldCanvas);
    await act(async () => oldRender.reject(new Error("RenderingCancelledException")));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(reportPDFPreviewFailure).not.toHaveBeenCalled();
  });

  it("ignores text failure from a page that is no longer displayed", async () => {
    const oldText = deferred<ReadableStreamReadResult<TextChunk>>();
    const reader = textReader("First page");
    reader.read.mockReset().mockReturnValueOnce(oldText.promise).mockResolvedValue({ done: true, value: undefined });
    first.streamTextContent.mockReturnValue({ getReader: () => reader });
    const view = render(<SigningPDF {...props} />);
    await waitFor(() => expect(first.streamTextContent).toHaveBeenCalled());
    view.rerender(<SigningPDF {...props} page={2} />);
    expect(await screen.findByText("Second page")).toBeInTheDocument();
    await act(async () => oldText.reject(new Error("stale text failure")));
    expect(reader.cancel).toHaveBeenCalledTimes(1);
    expect(reader.releaseLock).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText(/Page text is unavailable/)).not.toBeInTheDocument();
    expect(reportPDFPreviewFailure).not.toHaveBeenCalled();
  });

  it("does not restart loading or rendering when callback identities change", async () => {
    const view = render(<SigningPDF {...props} onReady={vi.fn()} onPageCount={vi.fn()} />);
    await screen.findByText("First page");
    const onReady = vi.fn();
    const onPageCount = vi.fn();
    view.rerender(<SigningPDF {...props} onReady={onReady} onPageCount={onPageCount} />);
    await waitFor(() => expect(onReady).toHaveBeenLastCalledWith(true));
    expect(onPageCount).toHaveBeenLastCalledWith(2);
    expect(pdfBytes).toHaveBeenCalledTimes(1);
    expect(first.render).toHaveBeenCalledTimes(1);
  });

  it("classifies document load failures and retries successfully", async () => {
    vi.mocked(pdfBytes).mockRejectedValueOnce(new Error("download failed"));
    render(<SigningPDF {...props} />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/PDF could not/);
    expect(reportPDFPreviewFailure).toHaveBeenCalledWith("load", expect.any(Error));
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("First page")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("reads every text chunk without requiring ReadableStream async iteration", async () => {
    const reader = textReader("First chunk");
    reader.read.mockResolvedValueOnce({ done: false, value: { items: [{ str: "Second chunk" }] } });
    const stream = { getReader: () => reader };
    expect(Symbol.asyncIterator in stream).toBe(false);
    first.streamTextContent.mockReturnValue(stream);
    render(<SigningPDF {...props} />);
    expect(await screen.findByText("First chunk Second chunk")).toBeInTheDocument();
    expect(first.getTextContent).not.toHaveBeenCalled();
    expect(reader.releaseLock).toHaveBeenCalledTimes(1);
    expect(reader.cancel).not.toHaveBeenCalled();
  });
});
