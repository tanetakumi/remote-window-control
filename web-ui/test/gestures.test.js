import test from "node:test";
import assert from "node:assert/strict";
import { attachGestureControls } from "../src/input/gestures.js";
import { attachTouchControlsUI } from "../src/input/touch-ui.js";

class Element extends EventTarget {
  rect = { left: 0, top: 0, width: 100, height: 100 };
  videoWidth = 100;
  videoHeight = 100;
  style = {};
  hidden = false;
  getBoundingClientRect() { return this.rect; }
}
function emit(target, type, fields = {}) {
  const event = new Event(type, { cancelable: true });
  Object.assign(event, fields);
  target.dispatchEvent(event);
  return event;
}
const finger = (x, y, identifier = 1) => ({ clientX: x, clientY: y, identifier });
const touch = (video, type, touches, changedTouches = []) => emit(video, type, { touches, changedTouches });
const tap = (video, point = finger(25, 75)) => {
  touch(video, "touchstart", [point]);
  touch(video, "touchend", [], [point]);
};

function setup(t, mode = "relative") {
  t.mock.timers.enable({ apis: ["setTimeout", "Date"], now: 1000 });
  const view = new EventTarget();
  view.setTimeout = (...args) => setTimeout(...args);
  view.clearTimeout = (...args) => clearTimeout(...args);
  const doc = new EventTarget();
  doc.defaultView = view;
  const video = new Element();
  video.ownerDocument = doc;
  const sent = [], positions = [];
  const controls = attachGestureControls(video, (command) => sent.push(command), {
    mode, onCursorChange: (point) => positions.push(point),
  });
  t.after(() => controls.cleanup());
  return { video, view, doc, sent, positions, controls };
}

test("relative motion moves the cursor; lifting and repositioning never warps or clicks", (t) => {
  const { video, sent, positions } = setup(t);
  touch(video, "touchstart", [finger(10, 10)]);
  touch(video, "touchmove", [finger(30, 40)]);
  touch(video, "touchend", [], [finger(30, 40)]);
  assert.deepEqual(sent, [{ type: "input.mouseMove", x: 0.7, y: 0.8 }]);
  tap(video, finger(90, 90));
  assert.deepEqual(sent.at(-1), { type: "input.tap", button: "left", x: 0.7, y: 0.8 });
  assert.deepEqual(positions.at(-1), { x: 0.7, y: 0.8 });
});

test("relative movement uses the displayed image size and clamps at its edges", (t) => {
  const { video, sent } = setup(t);
  video.videoWidth = 200;
  video.rect = { left: 40, top: 30, width: 200, height: 200 };
  touch(video, "touchstart", [finger(140, 130)]);
  touch(video, "touchmove", [finger(240, 180)]);
  assert.deepEqual(sent.at(-1), { type: "input.mouseMove", x: 1, y: 1 });
  touch(video, "touchmove", [finger(500, -300)]);
  touch(video, "touchend", [], [finger(500, -300)]);
  assert.deepEqual(sent.at(-1), { type: "input.mouseMove", x: 1, y: 0 });
  assert.equal(sent.some((c) => c.type === "input.tap"), false);
});

test("direct tap clicks the touched image position and ignores letterbox margins", (t) => {
  const { video, sent } = setup(t, "direct");
  tap(video);
  assert.deepEqual(sent, [{ type: "input.tap", button: "left", x: 0.25, y: 0.75 }]);
  video.videoWidth = 200;
  t.mock.timers.tick(301);
  tap(video, finger(50, 10));
  assert.equal(sent.length, 1);
  tap(video, finger(50, 50));
  assert.deepEqual(sent.at(-1), { type: "input.tap", button: "left", x: 0.5, y: 0.5 });
});

for (const mode of ["relative", "direct"]) {
  test(`${mode}: double-tap and hold drags with the left button and releases at the final position`, (t) => {
    const { video, sent } = setup(t, mode);
    tap(video, finger(25, 25));
    t.mock.timers.tick(50);
    touch(video, "touchstart", [finger(25, 25)]);
    touch(video, "touchmove", [finger(45, 55)]);
    touch(video, "touchend", [], [finger(50, 60)]);
    const origin = mode === "direct" ? { x: 0.25, y: 0.25 } : { x: 0.5, y: 0.5 };
    assert.deepEqual(sent.slice(0, 2), [
      { type: "input.tap", button: "left", ...origin },
      { type: "input.mouseDown", button: "left", ...origin },
    ]);
    assert.equal(sent.at(-1).type, "input.mouseUp");
    assert.equal(sent.at(-1).button, "left");
    const point = mode === "direct" ? { x: 0.5, y: 0.6 } : { x: 0.75, y: 0.85 };
    assert.ok(Math.abs(sent.at(-1).x - point.x) < 1e-12);
    assert.ok(Math.abs(sent.at(-1).y - point.y) < 1e-12);
    assert.equal(sent.filter((c) => c.type === "input.tap").length, 1);
  });

  test(`${mode}: long press right-clicks once; only relative mode continues moving the cursor`, (t) => {
    const { video, sent } = setup(t, mode);
    touch(video, "touchstart", [finger(25, 75)]);
    t.mock.timers.tick(450);
    touch(video, "touchmove", [finger(50, 50)]);
    touch(video, "touchend", [], [finger(55, 45)]);
    const point = mode === "direct" ? { x: 0.25, y: 0.75 } : { x: 0.5, y: 0.5 };
    const expected = [{ type: "input.tap", button: "right", ...point }];
    if (mode === "relative") {
      expected.push(
        { type: "input.mouseMove", x: 0.75, y: 0.25 },
        { type: "input.mouseMove", x: 0.8, y: 0.2 },
      );
    }
    assert.deepEqual(sent, expected);
  });

  test(`${mode}: a two-finger tap right-clicks once when fingers lift separately`, (t) => {
    const { video, sent } = setup(t, mode);
    touch(video, "touchstart", [finger(30, 50), finger(70, 50, 2)]);
    touch(video, "touchend", [finger(70, 50, 2)], [finger(30, 50)]);
    assert.deepEqual(sent, []);
    touch(video, "touchend", [], [finger(70, 50, 2)]);
    assert.deepEqual(sent, [{ type: "input.tap", button: "right", x: 0.5, y: 0.5 }]);
  });
}

test("one- and two-finger upward scrolling share direction and notch units", (t) => {
  const { video, sent, controls } = setup(t, "direct");
  touch(video, "touchstart", [finger(50, 80)]);
  touch(video, "touchmove", [finger(50, 60)]);
  touch(video, "touchend", [], [finger(50, 60)]);
  assert.deepEqual(sent, [{ type: "input.scroll", deltaY: 0.2, x: 0.5, y: 0.8 }]);
  controls.setMode("relative");
  touch(video, "touchstart", [finger(30, 80), finger(70, 80, 2)]);
  touch(video, "touchmove", [finger(30, 60), finger(70, 60, 2)]);
  touch(video, "touchend", [finger(70, 60, 2)], [finger(30, 60)]);
  touch(video, "touchend", [], [finger(70, 60, 2)]);
  assert.deepEqual(sent[1], sent[0]);
});

test("two-finger scrolling keeps working when reversing across its starting point", (t) => {
  const { video, sent } = setup(t);
  touch(video, "touchstart", [finger(30, 60), finger(70, 60, 2)]);
  touch(video, "touchmove", [finger(30, 40), finger(70, 40, 2)]);
  touch(video, "touchmove", [finger(30, 60), finger(70, 60, 2)]);
  assert.deepEqual(sent.map((c) => c.deltaY), [0.2, -0.2]);
});

test("fractional scroll movements accumulate into whole Windows wheel units", (t) => {
  const { video, sent } = setup(t, "direct");
  touch(video, "touchstart", [finger(50, 80)]);
  touch(video, "touchmove", [finger(50, 70)]);
  const initialUnits = sent.reduce((n, c) => n + Math.round(c.deltaY * 120), 0);
  for (let i = 1; i <= 10; i++) touch(video, "touchmove", [finger(50, 70 - i / 10)]);
  const units = sent.reduce((n, c) => n + Math.round(c.deltaY * 120), 0);
  assert.equal(units - initialUnits, 1);
  assert.equal(sent.some((c) => c.deltaX !== undefined), false);
});

test("pinch and third-finger gestures send neither wheel input nor stray taps", (t) => {
  const { video, sent } = setup(t);
  touch(video, "touchstart", [finger(40, 50), finger(60, 50, 2)]);
  touch(video, "touchmove", [finger(20, 70), finger(80, 70, 2)]);
  touch(video, "touchend", [], [finger(20, 70), finger(80, 70, 2)]);
  touch(video, "touchstart", [finger(30, 30), finger(40, 40, 2), finger(50, 50, 3)]);
  touch(video, "touchend", [finger(30, 30)], [finger(40, 40, 2), finger(50, 50, 3)]);
  touch(video, "touchmove", [finger(50, 50)]);
  touch(video, "touchend", [], [finger(50, 50)]);
  assert.deepEqual(sent, []);
  tap(video);
  assert.equal(sent.at(-1).type, "input.tap");
});

test("adding a second finger ends a left drag without a right-click on release", (t) => {
  const { video, sent } = setup(t);
  tap(video);
  touch(video, "touchstart", [finger(25, 75)]);
  touch(video, "touchstart", [finger(25, 75), finger(50, 75, 2)]);
  touch(video, "touchend", [], [finger(25, 75), finger(50, 75, 2)]);
  assert.deepEqual(sent.map((c) => [c.type, c.button]), [
    ["input.tap", "left"], ["input.mouseDown", "left"], ["input.mouseUp", "left"],
  ]);
});

test("a failed drag-release send stops cursor updates after synchronous cleanup", (t) => {
  const { video, controls } = setup(t, "direct");
  controls.cleanup();
  const sent = [], positions = [];
  const active = attachGestureControls(video, (command) => {
    sent.push(command);
    if (command.type === "input.mouseUp") active.cleanup();
  }, { mode: "direct", onCursorChange: (point) => positions.push(point) });
  t.after(() => active.cleanup());
  tap(video, finger(25, 25));
  touch(video, "touchstart", [finger(25, 25)]);
  const count = positions.length;
  touch(video, "touchstart", [finger(25, 25), finger(75, 75, 2)]);
  assert.equal(sent.at(-1).type, "input.mouseUp");
  assert.equal(positions.length, count, "no cursor update after cleanup from the send callback");
  touch(video, "touchend", [], [finger(25, 25), finger(75, 75, 2)]);
  assert.equal(sent.length, 3);
});

for (const reason of ["touchcancel", "blur", "visibility", "pagehide", "mode", "cleanup"]) {
  test(`${reason} releases a drag once and ignores the old contact sequence`, (t) => {
    const { video, view, doc, sent, controls } = setup(t);
    tap(video);
    touch(video, "touchstart", [finger(25, 75)]);
    touch(video, "touchmove", [finger(45, 55)]);
    if (reason === "touchcancel") touch(video, "touchcancel", [finger(45, 55)]);
    else if (reason === "mode") controls.setMode("direct");
    else if (reason === "cleanup") controls.cleanup();
    else if (reason === "visibility") { doc.hidden = true; emit(doc, "visibilitychange"); }
    else emit(view, reason);
    assert.equal(sent.at(-1).type, "input.mouseUp");
    const count = sent.length;
    touch(video, "touchmove", [finger(80, 20)]);
    touch(video, "touchend", [], [finger(80, 20)]);
    t.mock.timers.tick(500);
    controls.cleanup();
    assert.equal(sent.length, count);
  });
}

test("cancellation prevents a pending long press and new touches recover after release", (t) => {
  const { video, view, sent } = setup(t);
  touch(video, "touchstart", [finger(25, 75)]);
  emit(view, "blur");
  t.mock.timers.tick(500);
  touch(video, "touchend", [], [finger(25, 75)]);
  assert.deepEqual(sent, []);
  tap(video);
  assert.deepEqual(sent, [{ type: "input.tap", button: "left", x: 0.5, y: 0.5 }]);
});

test("mode changes preserve the cursor but clear double-tap history", (t) => {
  const { video, sent, controls } = setup(t, "direct");
  tap(video);
  controls.setMode("relative");
  tap(video, finger(90, 90));
  assert.deepEqual(sent[1], sent[0]);
  assert.equal(sent.some((c) => c.type === "input.mouseDown"), false);
});

test("physical mouse, pointer, wheel and compatibility click events never send remote input", (t) => {
  const { video, sent } = setup(t);
  for (const type of ["pointerdown", "pointermove", "pointerup", "mousedown", "mousemove", "mouseup", "wheel", "click"]) {
    emit(video, type, { pointerType: "mouse", pointerId: 1, buttons: 1, clientX: 25, clientY: 75, deltaY: 100 });
  }
  assert.deepEqual(sent, []);
  tap(video);
  emit(video, "click", { clientX: 25, clientY: 75 });
  assert.equal(sent.length, 1);
});

test("zero-sized images cannot start clicks or drags", (t) => {
  const { video, sent } = setup(t);
  video.rect.width = 0;
  tap(video);
  assert.deepEqual(sent, []);
});

test("a finger on another control cannot keep the video's contact sequence alive", (t) => {
  const { video, sent } = setup(t);
  const point = finger(25, 75);
  const outside = finger(200, 200, 2);
  emit(video, "touchstart", { touches: [point, outside], targetTouches: [point], changedTouches: [point] });
  emit(video, "touchend", { touches: [outside], targetTouches: [], changedTouches: [point] });
  assert.deepEqual(sent, [{ type: "input.tap", button: "left", x: 0.5, y: 0.5 }]);
});

function setupUI(t, saved, failStorage = false) {
  const { video, view, sent, controls } = setup(t);
  controls.cleanup();
  const store = new Map(saved === undefined ? [] : [["share-app.touch-mode", saved]]);
  Object.defineProperty(view, "localStorage", {
    get() {
      if (failStorage) throw new Error("storage disabled");
      return { getItem: (key) => store.get(key), setItem: (key, value) => store.set(key, value) };
    },
  });
  const elements = {
    videoElement: video, stageElement: new Element(), cursorElement: new Element(),
    modeElement: new Element(), hintElement: new Element(),
  };
  const cleanup = attachTouchControlsUI(elements, (c) => sent.push(c));
  t.after(cleanup);
  return { ...elements, view, store, sent, cleanup };
}

test("the mode selector restores and saves the two modes and changes cursor visibility", (t) => {
  const ui = setupUI(t, "direct");
  assert.equal(ui.modeElement.value, "direct");
  assert.equal(ui.cursorElement.hidden, true);
  tap(ui.videoElement);
  ui.modeElement.value = "relative";
  emit(ui.modeElement, "change");
  assert.equal(ui.store.get("share-app.touch-mode"), "relative");
  assert.equal(ui.cursorElement.hidden, false);
  assert.match(ui.hintElement.textContent, /move the cursor/);
  tap(ui.videoElement, finger(90, 90));
  assert.deepEqual(ui.sent[1], ui.sent[0]);
  ui.cleanup();
  assert.equal(ui.cursorElement.hidden, true);
});

for (const failStorage of [false, true]) {
  test(`invalid or unavailable storage defaults to relative pointer (storage blocked: ${failStorage})`, (t) => {
    const ui = setupUI(t, "mouse", failStorage);
    assert.equal(ui.modeElement.value, "relative");
    ui.modeElement.value = "direct";
    emit(ui.modeElement, "change");
    tap(ui.videoElement);
    assert.deepEqual(ui.sent, [{ type: "input.tap", button: "left", x: 0.25, y: 0.75 }]);
  });
}

test("cursor rendering follows letterboxing and video resize while preserving logical position", (t) => {
  const ui = setupUI(t);
  ui.videoElement.videoWidth = 200;
  ui.videoElement.rect = { left: 40, top: 30, width: 200, height: 200 };
  ui.stageElement.rect = { left: 40, top: 30, width: 200, height: 200 };
  emit(ui.videoElement, "resize");
  assert.equal(ui.cursorElement.style.left, "100px");
  assert.equal(ui.cursorElement.style.top, "100px");
  touch(ui.videoElement, "touchstart", [finger(140, 130)]);
  touch(ui.videoElement, "touchmove", [finger(160, 150)]);
  touch(ui.videoElement, "touchend", [], [finger(160, 150)]);
  assert.equal(ui.cursorElement.style.left, "120px");
  assert.equal(ui.cursorElement.style.top, "120px");
  ui.videoElement.rect = { left: 40, top: 30, width: 100, height: 100 };
  emit(ui.view, "resize");
  assert.equal(ui.cursorElement.style.left, "60px");
  assert.equal(ui.cursorElement.style.top, "60px");
});
