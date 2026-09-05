import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FieldOverlay } from "./SigningFields";
import type { SigningField } from "./signingApi";

const initial: SigningField = { id: "field", type: "signature", label: "Signature", page: 1, x: .2, y: .3, width: .4, height: .1, required: true };
const captured = new Set<number>();

beforeEach(() => {
  captured.clear();
  HTMLElement.prototype.setPointerCapture = vi.fn((id: number) => captured.add(id));
  HTMLElement.prototype.hasPointerCapture = vi.fn((id: number) => captured.has(id));
  HTMLElement.prototype.releasePointerCapture = vi.fn((id: number) => captured.delete(id));
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function setup(width = 600, height = 800, field = initial, pageSize = { width: 612, height: 792 }) {
  function Harness() {
    const [value, setValue] = useState(field);
    return <div className="signing-paper"><FieldOverlay field={value} editable selected onChange={setValue} pageSize={pageSize} /><output data-testid="position">{JSON.stringify(value)}</output></div>;
  }
  const result = render(<Harness />);
  vi.spyOn(result.container.querySelector(".signing-paper")!, "getBoundingClientRect").mockReturnValue({ x: 0, y: 0, top: 0, left: 0, right: width, bottom: height, width, height, toJSON() {} });
  return () => JSON.parse(screen.getByTestId("position").textContent!) as SigningField;
}

function pointer(target: HTMLElement, type: "down" | "move" | "up" | "cancel", x: number, y: number, pointerId = 1, pointerType = "mouse") {
  const event = { pointerId, pointerType, button: 0, clientX: x, clientY: y, isPrimary: true };
  if (type === "down") fireEvent.pointerDown(target, event);
  if (type === "move") fireEvent.pointerMove(target, event);
  if (type === "up") fireEvent.pointerUp(target, event);
  if (type === "cancel") fireEvent.pointerCancel(target, event);
}

describe("direct signing field placement", () => {
  it.each([[300, 400, "touch"], [600, 800, "mouse"], [1200, 1600, "mouse"]])("moves with rendered page dimensions %sx%s using %s", (width, height, kind) => {
    const current = setup(Number(width), Number(height));
    const move = screen.getByRole("button", { name: "Select Signature, page 1" });
    pointer(move, "down", 30, 40, 7, String(kind));
    pointer(move, "move", 30 + Number(width) * .1, 40 + Number(height) * .1, 7, String(kind));
    expect(current().x).toBeCloseTo(.3);
    expect(current().y).toBeCloseTo(.4);
    pointer(move, "up", 30 + Number(width) * .1, 40 + Number(height) * .1, 7, String(kind));
    expect(captured.has(7)).toBe(false);
    pointer(move, "move", 500, 500, 7, String(kind));
    expect(current().x).toBeCloseTo(.3);
  });

  it("resizes against the page edge without moving the fixed corner", () => {
    const current = setup();
    const corner = screen.getByRole("button", { name: "Resize Signature" });
    pointer(corner, "down", 100, 100);
    pointer(corner, "move", 2000, 2000);
    expect(current()).toMatchObject({ x: .2, y: .3, width: .8, height: .7 });
  });

  it("keeps the opposite corner fixed when resizing from top left", () => {
    const current = setup();
    const corner = screen.getByRole("button", { name: "Resize Signature from top left" });
    pointer(corner, "down", 100, 100);
    pointer(corner, "move", -1000, -1000);
    expect(current().x).toBe(0);
    expect(current().y).toBe(0);
    expect(current().width).toBeCloseTo(.6);
    expect(current().height).toBeCloseTo(.4);
    pointer(corner, "move", 2000, 2000);
    expect(current().width).toBeCloseTo(.05);
    expect(current().height).toBeCloseTo(15.6 / 792);
    expect(current().x + current().width).toBeCloseTo(.6);
    expect(current().y + current().height).toBeCloseTo(.4);
  });

  it("respects physical readable minimums on small pages", () => {
    const current = setup(300, 300, { ...initial, height: .3 }, { width: 72, height: 72 });
    const corner = screen.getByRole("button", { name: "Resize Signature" });
    pointer(corner, "down", 100, 100);
    pointer(corner, "move", -100, -100);
    expect(current().width).toBeCloseTo(20 / 72);
    expect(current().height).toBeCloseTo(15.6 / 72);
    expect(current().x).toBe(.2);
    expect(current().y).toBe(.3);
  });

  it("ignores other fingers and restores placement on pointer cancellation", () => {
    const current = setup();
    const move = screen.getByRole("button", { name: "Select Signature, page 1" });
    pointer(move, "down", 30, 40, 1, "touch");
    pointer(move, "down", 300, 400, 2, "touch");
    pointer(move, "move", 600, 800, 2, "touch");
    pointer(move, "up", 600, 800, 2, "touch");
    expect(current()).toEqual(initial);
    pointer(move, "move", 90, 120, 1, "touch");
    expect(current().x).toBeCloseTo(.3);
    pointer(move, "cancel", 90, 120, 1, "touch");
    expect(current()).toEqual(initial);
    expect(captured.has(1)).toBe(false);
  });

  it("stops a gesture after losing pointer capture", () => {
    const current = setup();
    const move = screen.getByRole("button", { name: "Select Signature, page 1" });
    pointer(move, "down", 30, 40);
    pointer(move, "move", 90, 120);
    fireEvent.lostPointerCapture(move, { pointerId: 1 });
    pointer(move, "move", 300, 400);
    expect(current().x).toBeCloseTo(.3);
    expect(current().y).toBeCloseTo(.4);
  });

  it("supports keyboard resizing from both corners with larger Shift steps", () => {
    const current = setup();
    fireEvent.keyDown(screen.getByRole("button", { name: "Resize Signature" }), { key: "ArrowRight", shiftKey: true });
    expect(current().width).toBeCloseTo(.42);
    fireEvent.keyDown(screen.getByRole("button", { name: "Resize Signature from top left" }), { key: "ArrowLeft" });
    expect(current().x).toBeCloseTo(.195);
    expect(current().width).toBeCloseTo(.425);
    expect(current().x + current().width).toBeCloseTo(.62);
  });
});
