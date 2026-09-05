import { useRef, type PointerEvent } from "react";
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
}: {
  field: SigningField;
  selected?: boolean;
  editable?: boolean;
  value?: string;
  onSelect?: () => void;
  onChange?: (field: SigningField) => void;
}) {
  const drag = useRef<{
    x: number;
    y: number;
    field: SigningField;
    width: number;
    height: number;
    resize: boolean;
  } | null>(null);
  function start(event: PointerEvent<HTMLButtonElement>, resize: boolean) {
    if (!editable || event.button !== 0) return;
    event.stopPropagation();
    const bounds = event.currentTarget
      .closest(".signing-paper")
      ?.getBoundingClientRect();
    if (!bounds) return;
    onSelect?.();
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = {
      x: event.clientX,
      y: event.clientY,
      field,
      width: bounds.width,
      height: bounds.height,
      resize,
    };
  }
  function move(event: PointerEvent<HTMLButtonElement>) {
    const origin = drag.current;
    if (!origin) return;
    const dx = (event.clientX - origin.x) / origin.width;
    const dy = (event.clientY - origin.y) / origin.height;
    onChange?.(
      boundedField({
        ...origin.field,
        ...(origin.resize
          ? { width: origin.field.width + dx, height: origin.field.height + dy }
          : { x: origin.field.x + dx, y: origin.field.y + dy }),
      }),
    );
  }
  return (
    <div
      className={`signing-field signing-field-${field.type}${selected ? " selected" : ""}${editable ? " editable" : ""}`}
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
            type="button"
            className="signing-field-move"
            aria-label={`Select ${field.label}, page ${field.page}`}
            aria-pressed={selected}
            onClick={onSelect}
            onPointerDown={(event) => start(event, false)}
            onPointerMove={move}
            onPointerUp={() => {
              drag.current = null;
            }}
            onPointerCancel={() => {
              drag.current = null;
            }}
            onKeyDown={(event) => {
              const step = event.shiftKey ? 0.02 : 0.005;
              const directions: Record<string, [number, number]> = {
                ArrowLeft: [-step, 0],
                ArrowRight: [step, 0],
                ArrowUp: [0, -step],
                ArrowDown: [0, step],
              };
              const direction = directions[event.key];
              if (direction) {
                event.preventDefault();
                onChange?.(
                  boundedField({
                    ...field,
                    x: field.x + direction[0],
                    y: field.y + direction[1],
                  }),
                );
              }
            }}
          >
            {field.label}
            {field.required ? " *" : ""}
          </button>
          <button
            type="button"
            className="signing-field-resize"
            aria-label={`Resize ${field.label}`}
            onPointerDown={(event) => start(event, true)}
            onPointerMove={move}
            onPointerUp={() => {
              drag.current = null;
            }}
            onPointerCancel={() => {
              drag.current = null;
            }}
            onClick={onSelect}
            onKeyDown={(event) => {
              const directions: Record<string, [number, number]> = {
                ArrowLeft: [-0.005, 0],
                ArrowRight: [0.005, 0],
                ArrowUp: [0, -0.005],
                ArrowDown: [0, 0.005],
              };
              const direction = directions[event.key];
              if (direction) {
                event.preventDefault();
                onChange?.(
                  boundedField({
                    ...field,
                    width: field.width + direction[0],
                    height: field.height + direction[1],
                  }),
                );
              }
            }}
          />
        </>
      ) : (
        <span title={value || field.label}>{value || field.label}</span>
      )}
    </div>
  );
}
