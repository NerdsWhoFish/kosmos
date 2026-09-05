import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  getDocument,
  GlobalWorkerOptions,
  type PDFDocumentProxy,
  type PDFPageProxy,
  type RenderTask,
} from "pdfjs-dist";
import workerURL from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { ErrorState, LoadingState } from "./States";
import { pdfBytes } from "../modules/signingApi";
import { reportPDFPreviewFailure } from "../telemetry";

GlobalWorkerOptions.workerSrc = workerURL;
type PageText = Awaited<ReturnType<PDFPageProxy["getTextContent"]>>;

export function SigningPDF({
  path,
  token,
  page,
  width,
  height,
  children,
  onReady,
  onPageCount,
}: {
  path: string;
  token?: string;
  page: number;
  width: number;
  height: number;
  children?: ReactNode;
  onReady?: (ready: boolean) => void;
  onPageCount?: (count: number) => void;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const generation = useRef(0);
  const [loaded, setLoaded] = useState<{ document: PDFDocumentProxy; generation: number }>();
  const [loadError, setLoadError] = useState("");
  const [renderError, setRenderError] = useState("");
  const [ready, setReady] = useState(false);
  const [text, setText] = useState("");
  const [textUnavailable, setTextUnavailable] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [zoom, setZoom] = useState(1);

  useEffect(() => {
    const abort = new AbortController();
    const currentGeneration = ++generation.current;
    let task: ReturnType<typeof getDocument> | undefined;
    setLoaded(undefined);
    setLoadError("");
    setReady(false);
    pdfBytes(path, token, abort.signal)
      .then(async (bytes) => {
        if (abort.signal.aborted) return;
        task = getDocument({
          data: bytes,
          cMapUrl: "/pdfjs/cmaps/",
          cMapPacked: true,
          standardFontDataUrl: "/pdfjs/standard_fonts/",
          wasmUrl: "/pdfjs/wasm/",
          iccUrl: "/pdfjs/iccs/",
        });
        const document = await task.promise;
        if (!abort.signal.aborted) {
          setLoaded({ document, generation: currentGeneration });
        }
      })
      .catch((error: unknown) => {
        if (!abort.signal.aborted) {
          reportPDFPreviewFailure("load", error);
          setLoadError(
            "The PDF could not be displayed. Retry or download the document to review it.",
          );
        }
      });
    return () => {
      abort.abort();
      void task?.destroy().catch(() => undefined);
    };
  }, [path, token, attempt]);

  useEffect(() => { onReady?.(ready); }, [ready, onReady]);
  useEffect(() => {
    if (loaded) onPageCount?.(loaded.document.numPages);
  }, [loaded, onPageCount]);

  useEffect(() => {
    let cancelled = false;
    let render: RenderTask | undefined;
    let textReader: ReadableStreamDefaultReader<PageText> | undefined;
    setReady(false);
    setRenderError("");
    setText("");
    setTextUnavailable(false);
    if (!loaded) return;
    loaded.document
      .getPage(page)
      .then(async (pdfPage) => {
        if (cancelled || !canvas.current) return;
        const viewport = pdfPage.getViewport({ scale: 1.75 });
        const target = canvas.current;
        target.width = viewport.width;
        target.height = viewport.height;
        render = pdfPage.render({ canvas: target, viewport });
        await render.promise;
        if (cancelled) return;
        setReady(true);
        try {
          // Safari lacks the stream async iterator used by getTextContent().
          // https://github.com/mozilla/pdf.js/issues/21557
          const reader = pdfPage.streamTextContent().getReader();
          textReader = reader;
          try {
            const chunks: string[] = [];
            while (!cancelled) {
              const { value, done } = await reader.read();
              if (cancelled || done) break;
              for (const item of (value as PageText).items) {
                if ("str" in item) chunks.push(item.str);
              }
            }
            if (!cancelled) setText(chunks.join(" "));
          } finally {
            reader.releaseLock();
            textReader = undefined;
          }
        } catch (error: unknown) {
          if (!cancelled) {
            reportPDFPreviewFailure("text", error);
            setTextUnavailable(true);
          }
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          reportPDFPreviewFailure("render", error);
          setRenderError(
            "This page could not finish rendering. Try again or download the document to review it.",
          );
        }
      });
    return () => {
      cancelled = true;
      render?.cancel();
      void textReader?.cancel().catch(() => undefined);
    };
  }, [loaded, page]);

  const error = loadError || renderError;
  const status = loadError ? "load-error" : renderError ? "render-error" : textUnavailable ? "text-unavailable" : ready ? "ready" : "loading";

  return (
    <section className="signing-preview" aria-label="Document preview" data-pdf-status={status}>
      <div className="signing-preview-tools">
        <span>PDF preview</span>
        <label>
          Zoom{" "}
          <select
            value={zoom}
            onChange={(event) => setZoom(Number(event.target.value))}
          >
            <option value={1}>Fit width</option>
            <option value={1.5}>150%</option>
            <option value={2}>200%</option>
          </select>
        </label>
      </div>
      {error && (
        <ErrorState
          message={error}
          retry={() => {
            setAttempt((value) => value + 1);
          }}
        />
      )}
      {!ready && !error && <LoadingState label="Rendering PDF" />}
      <div className="signing-paper-scroll">
        <div
          className="signing-paper"
          style={{
            aspectRatio: `${width} / ${height}`,
            width: `${zoom * 100}%`,
          }}
        >
          <canvas
            key={`${loaded?.generation ?? 0}:${page}`}
            ref={canvas}
            aria-label={`Document page ${page}`}
            role="img"
          />
          {ready && children}
        </div>
      </div>
      {textUnavailable && (
        <p className="signing-hint" role="status">
          Page text is unavailable. You can still review the PDF preview. Ask the sender for an accessible copy if you need one.
        </p>
      )}
      {text && (
        <details className="signing-page-text">
          <summary>Read page {page} as text</summary>
          <p>{text}</p>
        </details>
      )}
    </section>
  );
}
