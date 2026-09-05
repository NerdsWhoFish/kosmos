import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  getDocument,
  GlobalWorkerOptions,
  type PDFDocumentProxy,
  type RenderTask,
} from "pdfjs-dist";
import workerURL from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { ErrorState, LoadingState } from "./States";
import { pdfBytes } from "../modules/signingApi";

GlobalWorkerOptions.workerSrc = workerURL;

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
  const [pdf, setPDF] = useState<PDFDocumentProxy>();
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);
  const [text, setText] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [zoom, setZoom] = useState(1);

  useEffect(() => {
    const abort = new AbortController();
    let task: ReturnType<typeof getDocument> | undefined;
    setPDF(undefined);
    setError("");
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
          setPDF(document);
          onPageCount?.(document.numPages);
        }
      })
      .catch(() => {
        if (!abort.signal.aborted)
          setError(
            "The PDF could not be displayed. Retry or download the original to review it.",
          );
      });
    return () => {
      abort.abort();
      void task?.destroy();
    };
  }, [path, token, attempt, onPageCount]);

  useEffect(() => {
    let cancelled = false;
    let render: RenderTask | undefined;
    setReady(false);
    setText("");
    onReady?.(false);
    if (!pdf) return;
    pdf
      .getPage(page)
      .then(async (pdfPage) => {
        if (cancelled || !canvas.current) return;
        const viewport = pdfPage.getViewport({ scale: 1.75 });
        const target = canvas.current;
        target.width = viewport.width;
        target.height = viewport.height;
        render = pdfPage.render({ canvas: target, viewport });
        await render.promise;
        const content = await pdfPage.getTextContent();
        if (!cancelled) {
          setText(
            content.items
              .map((item) => ("str" in item ? item.str : ""))
              .join(" "),
          );
          setReady(true);
          onReady?.(true);
        }
      })
      .catch(() => {
        if (!cancelled)
          setError(
            "This page could not be displayed. Try loading the PDF again.",
          );
      });
    return () => {
      cancelled = true;
      render?.cancel();
    };
  }, [pdf, page, onReady]);

  return (
    <section className="signing-preview" aria-label="Document preview">
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
            setError("");
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
            ref={canvas}
            aria-label={`Document page ${page}`}
            role="img"
          />
          {ready && children}
        </div>
      </div>
      {text && (
        <details className="signing-page-text">
          <summary>Read page {page} as text</summary>
          <p>{text}</p>
        </details>
      )}
    </section>
  );
}
