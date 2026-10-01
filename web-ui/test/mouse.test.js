import test from "node:test";
import assert from "node:assert/strict";
import { attachMouseControls } from "../src/input/mouse.js";
import { attachGestureControls } from "../src/input/gestures.js";

class Video extends EventTarget {
  videoWidth = 100;
  videoHeight = 100;
  rect = { left: 0, top: 0, width: 100, height: 100 };
  ownerDocument = { defaultView: new EventTarget() };
  captured = new Set();
  getBoundingClientRect() { return this.rect; }
  setPointerCapture(id) { this.captured.add(id); }
  hasPointerCapture(id) { return this.captured.has(id); }
  releasePointerCapture(id) {
    this.captured.delete(id);
    emit(this, "lostpointercapture", { pointerId: id });
  }
}

function emit(target, type, fields = {}) {
  const event = new Event(type, { cancelable: true });
  Object.assign(event, fields);
  target.dispatchEvent(event);
  return event;
}

function pointer(video, type, fields = {}) {
  return emit(video, type, {
    pointerId: 1, pointerType: "mouse", clientX: 25, clientY: 75,
    button: -1, buttons: 0, ...fields,
  });
}

function setup() {
  const video = new Video();
  const sent = [];
  const dispose = attachMouseControls(video, (command) => sent.push(command));
  return { video, sent, dispose };
}

test("mouse hover, left press, drag and release send normalized input", () => {
  const { video, sent, dispose } = setup();
  pointer(video, "pointermove");
  const down = pointer(video, "pointerdown", { button: 0, buttons: 1 });
  assert.equal(down.defaultPrevented, true);
  assert.equal(video.hasPointerCapture(1), true);
  pointer(video, "pointermove", { buttons: 1, clientX: 80, clientY: 90 });
  pointer(video, "pointerup", { button: 0, clientX: 80, clientY: 90 });
  assert.deepEqual(sent, [
    { type: "input.mouseMove", x: 0.25, y: 0.75 },
    { type: "input.mouseDown", button: "left", x: 0.25, y: 0.75 },
    { type: "input.mouseMove", x: 0.8, y: 0.9 },
    { type: "input.mouseUp", button: "left", x: 0.8, y: 0.9 },
  ]);
  assert.equal(video.hasPointerCapture(1), false);
  dispose();
  assert.equal(sent.length, 4, "cleanup never repeats an already released button");
});

test("right clicks suppress the browser menu and preserve remote down/up", () => {
  const { video, sent, dispose } = setup();
  pointer(video, "pointerdown", { button: 2, buttons: 2 });
  assert.equal(emit(video, "contextmenu").defaultPrevented, true);
  assert.equal(emit(video, "dragstart").defaultPrevented, true);
  pointer(video, "pointerup", { button: 2 });
  assert.deepEqual(sent.map(({ type, button }) => [type, button]), [
    ["input.mouseDown", "right"], ["input.mouseUp", "right"],
  ]);
  dispose();
});

test("mouse coordinates follow the letterboxed video and ignore the margins", () => {
  const { video, sent, dispose } = setup();
  video.videoWidth = 200;
  video.rect = { left: 40, top: 30, width: 200, height: 200 };
  pointer(video, "pointermove", { clientX: 140, clientY: 50 });
  pointer(video, "pointerdown", { button: 0, buttons: 1, clientX: 140, clientY: 50 });
  emit(video, "wheel", { clientX: 140, clientY: 50, deltaY: 100 });
  assert.deepEqual(sent, []);
  pointer(video, "pointerdown", { button: 0, buttons: 1, clientX: 140, clientY: 130 });
  // Pointer capture keeps delivering the drag even outside the video element.
  pointer(video, "pointermove", { buttons: 1, clientX: 300, clientY: -10 });
  pointer(video, "pointerup", { button: 0, clientX: 300, clientY: -10 });
  assert.deepEqual(sent, [
    { type: "input.mouseDown", button: "left", x: 0.5, y: 0.5 },
    { type: "input.mouseMove", x: 1, y: 0 },
    { type: "input.mouseUp", button: "left", x: 1, y: 0 },
  ]);
  dispose();
});

test("a zero-sized video sends no invalid coordinates", () => {
  const { video, sent, dispose } = setup();
  video.rect.width = 0;
  pointer(video, "pointermove");
  pointer(video, "pointerdown", { button: 0, buttons: 1 });
  emit(video, "wheel", { clientX: 25, clientY: 75, deltaY: 100 });
  assert.deepEqual(sent, []);
  dispose();
});

test("chorded left/right button changes are handled on pointermove", () => {
  const { video, sent, dispose } = setup();
  pointer(video, "pointerdown", { button: 0, buttons: 1 });
  pointer(video, "pointermove", { button: 2, buttons: 3 });
  pointer(video, "pointermove", { button: 0, buttons: 2 });
  pointer(video, "pointerup", { button: 2 });
  assert.deepEqual(sent.filter((c) => c.button).map(({ type, button }) => [type, button]), [
    ["input.mouseDown", "left"], ["input.mouseDown", "right"],
    ["input.mouseUp", "left"], ["input.mouseUp", "right"],
  ]);
  dispose();
});

test("cancellation, capture loss, blur and disposal release every button at the last position", () => {
  for (const reason of ["pointercancel", "lostpointercapture", "blur", "dispose"]) {
    const { video, sent, dispose } = setup();
    pointer(video, "pointerdown", { buttons: 3 });
    pointer(video, "pointermove", { buttons: 3, clientX: 80, clientY: 90 });
    // Losing a touch's capture must not cancel an unrelated mouse drag.
    emit(video, "lostpointercapture", { pointerId: 2 });
    assert.equal(sent.length, 3);
    if (reason === "dispose") dispose();
    else if (reason === "blur") emit(video.ownerDocument.defaultView, reason);
    else emit(video, reason, { pointerId: 1 });
    assert.deepEqual(sent.slice(-2), [
      { type: "input.mouseUp", button: "left", x: 0.8, y: 0.9 },
      { type: "input.mouseUp", button: "right", x: 0.8, y: 0.9 },
    ], reason);
    dispose();
    assert.equal(sent.length, 5, reason);
    pointer(video, "pointerdown", { button: 0, buttons: 1 });
    emit(video, "wheel", { clientX: 25, clientY: 75, deltaY: 100 });
    assert.equal(sent.length, 5, "disposed controls send nothing");
  }
});

test("buttons held outside the video or after blur never begin a remote drag", () => {
  const { video, sent, dispose } = setup();
  pointer(video, "pointermove", { buttons: 1 });
  pointer(video, "pointerup", { button: 0 });
  pointer(video, "pointerdown", { button: 1, buttons: 4 });
  assert.deepEqual(sent, []);
  pointer(video, "pointerdown", { button: 0, buttons: 1 });
  emit(video.ownerDocument.defaultView, "blur");
  const released = sent.length;
  pointer(video, "pointermove", { buttons: 1 });
  pointer(video, "pointerup", { button: 0 });
  assert.equal(sent.length, released);
  dispose();
});

test("wheel units are converted to notches at the mouse position", () => {
  const { video, sent, dispose } = setup();
  for (const [deltaMode, deltaY, expected] of [[0, 100, 1], [1, 3, 1], [2, 1, 1], [0, -50, -0.5], [0, 0.5, 0.005]]) {
    const event = emit(video, "wheel", { clientX: 25, clientY: 75, deltaMode, deltaY });
    assert.equal(event.defaultPrevented, true);
    assert.deepEqual(sent.at(-1), { type: "input.scroll", deltaY: expected, x: 0.25, y: 0.75 });
  }
  assert.equal(emit(video, "wheel", { clientX: 25, clientY: 75, deltaY: 100, ctrlKey: true }).defaultPrevented, false);
  assert.equal(sent.length, 5, "pinch zoom remains local");
  dispose();
});

test("touch input still sends one tap without duplicate mouse commands", () => {
  globalThis.window = { setTimeout, clearTimeout };
  const { video, sent, dispose } = setup();
  const releaseTouch = attachGestureControls(video, (command) => sent.push(command), () => {});
  const touch = { clientX: 25, clientY: 75 };
  pointer(video, "pointerdown", { pointerType: "touch", button: 0, buttons: 1 });
  emit(video, "touchstart", { touches: [touch], changedTouches: [touch] });
  pointer(video, "pointermove", { pointerType: "touch", buttons: 1 });
  pointer(video, "pointerup", { pointerType: "touch", button: 0 });
  emit(video, "touchend", { touches: [], changedTouches: [touch] });
  emit(video, "mousedown", { button: 0, buttons: 1, ...touch });
  emit(video, "mouseup", { button: 0, buttons: 0, ...touch });
  emit(video, "click", touch);
  assert.deepEqual(sent, [{ type: "input.tap", button: "left", x: 0.25, y: 0.75 }]);
  releaseTouch();
  dispose();
});
