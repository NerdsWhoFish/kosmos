import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import { boundedField, type SigningField, type SigningRequest } from "./signingApi";

export function Status({ request }: { request: SigningRequest }) {
  const expired =
    request.status === "pending" &&
    !!request.expiresAt &&
    new Date(request.expiresAt).getTime() <= Date.now();
  const label = expired
    ? "Expired"
    : {
        draft: "Draft",
        pending: "Awaiting signature",
        completed: "Completed",
        revoked: "Revoked",
      }[request.status];
  return (
    <span
      className={`signing-status signing-status-${expired ? "revoked" : request.status}`}
    >
      {label}
    </span>
  );
}

export function PageControls({
  page,
  count,
  onChange,
}: {
  page: number;
  count: number;
  onChange: (page: number) => void;
}) {
  return (
    <nav className="signing-page-controls" aria-label="PDF pages">
      <button
        className="secondary-button"
        disabled={page === 1}
        onClick={() => onChange(page - 1)}
      >
        Previous
      </button>
      <label>
        Page{" "}
        <select
          value={page}
          onChange={(event) => onChange(Number(event.target.value))}
        >
          {Array.from({ length: count }, (_, index) => (
            <option key={index} value={index + 1}>
              {index + 1} of {count}
            </option>
          ))}
        </select>
      </label>
      <button
        className="secondary-button"
        disabled={page === count}
        onClick={() => onChange(page + 1)}
      >
        Next
      </button>
    </nav>
  );
}

export function FieldOverlay({
  field,
  selected,
  editable,
  value,
  onSelect,
  onChange,
  pageSize,
  focusOnMount,
}: {
  field: SigningField;
  selected?: boolean;
  editable?: boolean;
  value?: string;
  onSelect?: () => void;
  onChange?: (field: SigningField) => void;
  pageSize?: { width: number; height: number };
  focusOnMount?: boolean;
}) {
  const moveButton = useRef<HTMLButtonElement>(null);
  const [interacting, setInteracting] = useState(false);
  type Mode = "move" | "nw" | "se";
  const drag = useRef<{
    pointerID: number;
    x: number;
    y: number;
    field: SigningField;
    width: number;
    height: number;
    mode: Mode;
  } | null>(null);

  useEffect(() => {
    if (focusOnMount) {
      moveButton.current?.focus({ preventScroll: true });
      moveButton.current?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    }
  }, [focusOnMount]);

  function transform(origin: SigningField, mode: Mode, dx: number, dy: number) {
    if (mode === "move") {
      return boundedField(
        { ...origin, x: origin.x + dx, y: origin.y + dy },
        pageSize,
      );
    }
    const minimum = boundedField({ ...origin, width: 0, height: 0 }, pageSize);
    if (mode === "se") {
      return {
        ...origin,
        width: Math.max(minimum.width, Math.min(1 - origin.x, origin.width + dx)),
        height: Math.max(minimum.height, Math.min(1 - origin.y, origin.height + dy)),
      };
    }
    const right = origin.x + origin.width;
    const bottom = origin.y + origin.height;
    const x = Math.max(0, Math.min(right - minimum.width, origin.x + dx));
    const y = Math.max(0, Math.min(bottom - minimum.height, origin.y + dy));
    return { ...origin, x, y, width: right - x, height: bottom - y };
  }
  function start(event: PointerEvent<HTMLButtonElement>, mode: Mode) {
    if (!editable || event.button !== 0 || drag.current) return;
    event.preventDefault();
    event.stopPropagation();
    const bounds = event.currentTarget
      .closest(".signing-paper")
      ?.getBoundingClientRect();
    if (!bounds || bounds.width <= 0 || bounds.height <= 0) return;
    onSelect?.();
    event.currentTarget.focus({ preventScroll: true });
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = {
      pointerID: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      field,
      width: bounds.width,
      height: bounds.height,
      mode,
    };
    setInteracting(true);
  }
  function move(event: PointerEvent<HTMLButtonElement>) {
    const origin = drag.current;
    if (!origin || event.pointerId !== origin.pointerID) return;
    const dx = (event.clientX - origin.x) / origin.width;
    const dy = (event.clientY - origin.y) / origin.height;
    onChange?.(transform(origin.field, origin.mode, dx, dy));
  }
  function finish(event: PointerEvent<HTMLButtonElement>, cancel = false) {
    const origin = drag.current;
    if (!origin || event.pointerId !== origin.pointerID) return;
    if (cancel) onChange?.(origin.field);
    else move(event);
    drag.current = null;
    setInteracting(false);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }
  function lostCapture(event: PointerEvent<HTMLButtonElement>) {
    if (drag.current?.pointerID !== event.pointerId) return;
    drag.current = null;
    setInteracting(false);
  }
  function keyboard(event: KeyboardEvent<HTMLButtonElement>, mode: Mode) {
    const step = event.shiftKey ? 0.02 : 0.005;
    const direction = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    }[event.key];
    if (!direction) return;
    event.preventDefault();
    onSelect?.();
    onChange?.(transform(field, mode, direction[0], direction[1]));
  }
  return (
    <div
      className={`signing-field signing-field-${field.type}${selected ? " selected" : ""}${editable ? " editable" : ""}${interacting ? " interacting" : ""}`}
      style={{
        left: `${field.x * 100}%`,
        top: `${field.y * 100}%`,
        width: `${field.width * 100}%`,
        height: `${field.height * 100}%`,
      }}
    >
      {editable ? (
        <>
          <button
            ref={moveButton}
            type="button"
            className="signing-field-move"
            aria-label={`Select ${field.label}, page ${field.page}`}
            aria-pressed={selected}
            title="Drag to move. Arrow keys adjust position; Shift moves further."
            onFocus={onSelect}
            onClick={onSelect}
            onPointerDown={(event) => start(event, "move")}
            onPointerMove={move}
            onPointerUp={(event) => finish(event)}
            onPointerCancel={(event) => finish(event, true)}
            onLostPointerCapture={lostCapture}
            onKeyDown={(event) => keyboard(event, "move")}
          >
            {field.label}
            {field.required ? " *" : ""}
          </button>
          {(["nw", "se"] as const).map((corner) => (
            <button
              key={corner}
              type="button"
              className={`signing-field-resize signing-field-resize-${corner}`}
              aria-label={`Resize ${field.label}${corner === "nw" ? " from top left" : ""}`}
              title={`Drag the ${corner === "nw" ? "top left" : "bottom right"} corner to resize. Arrow keys also resize.`}
              onFocus={onSelect}
              onPointerDown={(event) => start(event, corner)}
              onPointerMove={move}
              onPointerUp={(event) => finish(event)}
              onPointerCancel={(event) => finish(event, true)}
              onLostPointerCapture={lostCapture}
              onClick={onSelect}
              onKeyDown={(event) => keyboard(event, corner)}
            >
              <span aria-hidden="true" />
            </button>
          ))}
        </>
      ) : (
        <span title={value || field.label}>{value || field.label}</span>
      )}
    </div>
  );
}
