import test from "node:test";
import assert from "node:assert/strict";
import { attachSpecialKeysPalette } from "../src/input/special-keys-palette.js";

class Element extends EventTarget {
  hidden = true;
  disabled = true;
  style = {};
  attributes = {};
  captured = new Set();
  offsetWidth = 180;
  offsetHeight = 58;
  rect = { left: 0, top: 55, width: 390, height: 789 };
  classList = {
    add() {}, remove() {}, toggle() {},
  };
  focus() {}
  setAttribute(name, value) { this.attributes[name] = value; }
  getBoundingClientRect() { return this.rect; }
  setPointerCapture(id) { this.captured.add(id); }
  hasPointerCapture(id) { return this.captured.has(id); }
  releasePointerCapture(id) { this.captured.delete(id); }
}

function emit(target, type, fields = {}) {
  const event = new Event(type, { cancelable: true });
  Object.assign(event, fields);
  target.dispatchEvent(event);
  return event;
}

function fixture(sendControl) {
  const view = Object.assign(new EventTarget(), {
    innerWidth: 390, innerHeight: 844,
    getComputedStyle: () => ({ getPropertyValue: () => "0px" }),
  });
  const controls = Object.fromEntries([
    "buttonElement", "paletteElement", "stageElement", "moveHandle", "keyButton", "shiftEnterButton",
  ].map(name => [name, new Element()]));
  controls.keyButtons = [
    { element: controls.keyButton, keys: ["Backspace"] },
    { element: controls.shiftEnterButton, keys: ["Shift", "Enter"] },
  ];
  controls.stageElement.ownerDocument = { defaultView: view };
  const sent = [];
  const send = sendControl ?? (command => { sent.push(command); return true; });
  const palette = attachSpecialKeysPalette(controls, send);
  return { controls, view, sent, send, palette };
}

test("only an active Backspace click sends one press and release; closing disables stale clicks", () => {
  const { controls: c, sent, palette } = fixture();
  assert.equal(c.buttonElement.disabled, false);
  assert.equal(c.paletteElement.hidden, true);
  assert.equal(c.buttonElement.attributes["aria-pressed"], "false");
  emit(c.keyButton, "click");
  emit(c.buttonElement, "click");
  assert.equal(c.paletteElement.hidden, false);
  assert.equal(c.buttonElement.attributes["aria-expanded"], "true");
  assert.equal(c.buttonElement.attributes["aria-pressed"], "true");
  emit(c.keyButton, "pointerdown", { pointerId: 1, isPrimary: true, button: 0 });
  emit(c.keyButton, "pointercancel", { pointerId: 1 });
  emit(c.keyButton, "click", { detail: 1 });
  emit(c.keyButton, "pointerdown", { pointerId: 2, isPrimary: true, button: 0 });
  emit(c.keyButton, "pointerup", { pointerId: 2, clientX: -1, clientY: 100 });
  emit(c.keyButton, "click", { detail: 1 });
  assert.deepEqual(sent, []);
  emit(c.keyButton, "click", { detail: 0 });
  assert.deepEqual(sent, [
    { type: "input.keyDown", key: "Backspace" },
    { type: "input.keyUp", key: "Backspace" },
  ]);
  assert.equal(c.paletteElement.hidden, false);
  emit(c.buttonElement, "click");
  emit(c.keyButton, "click");
  assert.equal(c.paletteElement.hidden, true);
  assert.equal(c.buttonElement.attributes["aria-pressed"], "false");
  assert.equal(sent.length, 2);
  palette.cleanup();
});

test("Shift+Enter holds Shift around Enter and releases in reverse order", () => {
  const { controls: c, sent, palette } = fixture();
  emit(c.buttonElement, "click");
  emit(c.shiftEnterButton, "click");
  assert.deepEqual(sent, [
    { type: "input.keyDown", key: "Shift" },
    { type: "input.keyDown", key: "Enter" },
    { type: "input.keyUp", key: "Enter" },
    { type: "input.keyUp", key: "Shift" },
  ]);
  palette.cleanup();
});

test("text input suspends the palette and retains its position across temporary resizing", () => {
  const { controls: c, view, sent, palette } = fixture();
  emit(c.buttonElement, "click");
  emit(c.moveHandle, "pointerdown", { pointerId: 1, isPrimary: true, button: 0, clientX: 150, clientY: 100 });
  emit(c.moveHandle, "pointermove", { pointerId: 1, clientX: 50, clientY: 200 });
  const saved = c.paletteElement.style.transform;
  palette.setTextInputActive(true);
  assert.equal(c.paletteElement.hidden, true);
  assert.equal(c.buttonElement.attributes["aria-pressed"], "false");
  assert.equal(c.moveHandle.captured.size, 0);
  emit(c.keyButton, "click");
  c.stageElement.rect.height = 80;
  emit(view, "resize");
  palette.setTextInputActive(false);
  assert.equal(c.paletteElement.hidden, false);
  assert.equal(c.buttonElement.attributes["aria-pressed"], "true");
  assert.notEqual(c.paletteElement.style.transform, saved);
  c.stageElement.rect.height = 789;
  emit(view, "resize");
  assert.equal(c.paletteElement.style.transform, saved);
  emit(c.buttonElement, "click");
  palette.setTextInputActive(true);
  palette.setTextInputActive(false);
  assert.equal(c.paletteElement.hidden, true);
  assert.deepEqual(sent, []);
  palette.cleanup();
});

test("dragging tracks one pointer, stays inside the stage, and cancellation never sends a key", () => {
  const { controls: c, sent, palette } = fixture();
  emit(c.buttonElement, "click");
  const initial = c.paletteElement.style.transform;
  emit(c.moveHandle, "pointerdown", { pointerId: 2, isPrimary: false, button: 0, clientX: 0, clientY: 0 });
  emit(c.moveHandle, "pointermove", { pointerId: 2, clientX: 500, clientY: 500 });
  assert.equal(c.paletteElement.style.transform, initial);
  emit(c.moveHandle, "pointerdown", { pointerId: 1, isPrimary: true, button: 0, clientX: 100, clientY: 100 });
  emit(c.moveHandle, "pointermove", { pointerId: 2, clientX: 500, clientY: 500 });
  assert.equal(c.paletteElement.style.transform, initial);
  emit(c.moveHandle, "pointermove", { pointerId: 1, clientX: -1000, clientY: 2000 });
  assert.equal(c.paletteElement.style.transform, "translate3d(8px, 723px, 0)");
  emit(c.moveHandle, "pointercancel", { pointerId: 1 });
  assert.equal(c.moveHandle.captured.size, 0);
  emit(c.moveHandle, "pointermove", { pointerId: 1, clientX: 100, clientY: 100 });
  assert.equal(c.paletteElement.style.transform, "translate3d(8px, 723px, 0)");
  const arrow = emit(c.moveHandle, "keydown", { key: "ArrowRight" });
  assert.equal(arrow.defaultPrevented, true);
  assert.equal(c.paletteElement.style.transform, "translate3d(18px, 723px, 0)");
  assert.deepEqual(sent, []);
  palette.cleanup();
});

test("failed sending and synchronous disconnect never retry or leave the palette enabled", () => {
  for (const failure of ["press", "release", "disconnect"]) {
    const sent = [];
    let palette;
    const f = fixture(command => {
      sent.push(command);
      if (failure === "disconnect") palette.cleanup();
      return failure === "press" ? false : command.type !== "input.keyUp";
    });
    palette = f.palette;
    emit(f.controls.buttonElement, "click");
    emit(f.controls.keyButton, "click");
    assert.equal(sent.length, failure === "release" ? 2 : 1);
    if (failure === "disconnect") {
      assert.equal(f.controls.paletteElement.hidden, true);
      assert.equal(f.controls.keyButton.disabled, true);
    }
    palette.cleanup();
  }
});

test("cleanup removes old listeners and reconnection starts closed at the initial position", () => {
  const { controls: c, view, sent, send, palette } = fixture();
  emit(c.buttonElement, "click");
  const initial = c.paletteElement.style.transform;
  emit(c.moveHandle, "keydown", { key: "ArrowDown" });
  palette.cleanup();
  palette.cleanup();
  palette.setTextInputActive(false);
  emit(c.buttonElement, "click");
  emit(c.keyButton, "click");
  emit(view, "resize");
  assert.equal(c.paletteElement.hidden, true);
  assert.equal(c.buttonElement.disabled, true);
  assert.deepEqual(sent, []);
  const reconnected = attachSpecialKeysPalette(c, send);
  assert.equal(c.paletteElement.hidden, true);
  emit(c.buttonElement, "click");
  assert.equal(c.paletteElement.style.transform, initial);
  emit(c.keyButton, "click");
  assert.equal(sent.length, 2);
  reconnected.cleanup();
});
