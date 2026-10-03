import test from "node:test";
import assert from "node:assert/strict";
import { attachViewportSync } from "../src/input/viewport.js";

test("editor height changes are suspended, closing refreshes size, and rotation still syncs", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const view = Object.assign(new EventTarget(), {
    setTimeout: (...args) => setTimeout(...args),
    clearTimeout: (...args) => clearTimeout(...args),
    devicePixelRatio: 2,
    visualViewport: new EventTarget(),
  });
  globalThis.window = view;
  globalThis.document = new EventTarget();
  globalThis.screen = { orientation: new EventTarget() };
  globalThis.cancelAnimationFrame = () => {};
  let active = false;
  let bounds = { width: 390, height: 700 };
  const sent = [];
  const viewport = attachViewportSync((command) => sent.push(command), {
    isSuspended: () => active,
    targetElement: { getBoundingClientRect: () => bounds },
  });
  t.after(() => viewport.cleanup());
  assert.deepEqual(sent, [{ type: "viewport.resize", width: 390, height: 700, devicePixelRatio: 2 }]);
  active = true;
  bounds = { width: 390, height: 380 };
  view.visualViewport.dispatchEvent(new Event("resize"));
  t.mock.timers.tick(120);
  assert.equal(sent.length, 1);
  bounds = { width: 812, height: 310 };
  view.dispatchEvent(new Event("orientationchange"));
  t.mock.timers.tick(120);
  assert.equal(sent.at(-1).width, 812);
  active = false;
  bounds = { width: 812, height: 340 };
  viewport.refresh();
  t.mock.timers.tick(120);
  assert.equal(sent.at(-1).height, 340);
  viewport.cleanup();
  const count = sent.length;
  bounds = { width: 390, height: 700 };
  view.dispatchEvent(new Event("resize"));
  viewport.refresh();
  t.mock.timers.tick(2000);
  assert.equal(sent.length, count);
});

test("viewport rejected during mode switching is retried after confirmation", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  globalThis.window = Object.assign(new EventTarget(), { setTimeout, clearTimeout, innerWidth: 375, innerHeight: 667, devicePixelRatio: 1 });
  globalThis.document = new EventTarget();
  globalThis.screen = {};
  globalThis.cancelAnimationFrame = () => {};
  let switching = true;
  const accepted = [];
  const viewport = attachViewportSync(c => { if (switching) return false; accepted.push(c); return true; });
  t.after(() => viewport.cleanup());
  assert.equal(accepted.length, 0);
  switching = false;
  viewport.refresh();
  t.mock.timers.tick(120);
  assert.equal(accepted.length, 1);
  viewport.refresh();
  t.mock.timers.tick(120);
  assert.equal(accepted.length, 1);
});
