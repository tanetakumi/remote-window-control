import test from "node:test";
import assert from "node:assert/strict";
import { attachSettingsScreen } from "../src/ui/settings-screen.js";

class Element extends EventTarget {
  value = "";
  textContent = "";
  disabled = false;
  hidden = true;
  attributes = {};
  classes = new Set();
  classList = { toggle: (name, on) => (on ? this.classes.add(name) : this.classes.delete(name)) };
  setAttribute(name, value) { this.attributes[name] = value; }
}

function createScreen(api) {
  const controls = {
    fpsInput: new Element(), fpsValue: new Element(),
    crfInput: new Element(), crfValue: new Element(),
    scaleInput: new Element(), scaleValue: new Element(),
    scrollInput: new Element(), scrollValue: new Element(),
    saveButton: new Element(), statusElement: new Element(),
    noticeElement: new Element(), dismissButton: new Element(),
  };
  return { controls, screen: attachSettingsScreen(controls, api) };
}

const click = (element) => element.dispatchEvent(new Event("click"));
const settle = () => new Promise((resolve) => setImmediate(resolve));

test("opening loads the stored settings and enables Save", async () => {
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 12, crf: 24, maxScale: 1.5, scrollSensitivity: 2 }),
    save: async () => assert.fail("not saved"),
  });
  assert.equal(controls.saveButton.disabled, true);
  await screen.open();
  assert.equal(controls.fpsInput.value, "12");
  assert.equal(controls.crfInput.value, "24");
  assert.equal(controls.scaleInput.value, "1.5");
  assert.equal(controls.scrollInput.value, "2");
  assert.equal(controls.scrollValue.textContent, "2×");
  assert.equal(controls.saveButton.disabled, false);
  assert.equal(controls.statusElement.textContent, "");
  assert.equal(controls.noticeElement.hidden, true);
});

test("a failed load reports the error and keeps Save disabled", async () => {
  const { controls, screen } = createScreen({
    load: async () => { throw new Error("host unreachable"); },
    save: async () => assert.fail("not saved"),
  });
  await screen.open();
  assert.equal(controls.statusElement.textContent, "host unreachable");
  assert.equal(controls.statusElement.classes.has("has-error"), true);
  assert.equal(controls.noticeElement.hidden, false);
  assert.equal(controls.saveButton.disabled, true);
  click(controls.saveButton);
});

test("Save sends the edited values and shows a temporary floating notice", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const saved = [];
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31, maxScale: 2, scrollSensitivity: 1 }),
    save: async (settings) => { saved.push(settings); return settings; },
  });
  await screen.open();
  controls.fpsInput.value = "20";
  controls.crfInput.value = "0";
  controls.scaleInput.value = "0.75";
  controls.scrollInput.value = "2.5";
  controls.fpsInput.dispatchEvent(new Event("input"));
  click(controls.saveButton);
  assert.equal(controls.saveButton.disabled, true);
  click(controls.saveButton);
  await settle();
  assert.deepEqual(saved, [{ fps: 20, crf: 0, maxScale: 0.75, scrollSensitivity: 2.5 }]);
  assert.match(controls.statusElement.textContent, /^Saved/);
  assert.match(controls.statusElement.textContent, /Reconnect to apply/);
  assert.match(controls.statusElement.textContent, /window scale.*viewport changes/);
  assert.equal(controls.noticeElement.hidden, false);
  assert.equal(controls.statusElement.classes.has("has-error"), false);
  assert.equal(controls.saveButton.disabled, false);
  assert.equal(controls.fpsValue.textContent, "20");
  t.mock.timers.tick(4999);
  assert.equal(controls.noticeElement.hidden, false);
  t.mock.timers.tick(1);
  assert.equal(controls.noticeElement.hidden, true);
  assert.equal(controls.statusElement.textContent, "");
});

test("the sliders show their value while moving", async () => {
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31, maxScale: 2, scrollSensitivity: 1 }),
    save: async () => assert.fail("not saved"),
  });
  await screen.open();
  assert.equal(controls.fpsValue.textContent, "8");
  assert.equal(controls.crfValue.textContent, "31");
  assert.equal(controls.scaleValue.textContent, "2");
  assert.equal(controls.scrollValue.textContent, "1×");
  controls.fpsInput.value = "15";
  controls.fpsInput.dispatchEvent(new Event("input"));
  controls.crfInput.value = "40";
  controls.crfInput.dispatchEvent(new Event("input"));
  controls.scaleInput.value = "3.5";
  controls.scaleInput.dispatchEvent(new Event("input"));
  controls.scrollInput.value = "2.25";
  controls.scrollInput.dispatchEvent(new Event("input"));
  assert.equal(controls.fpsValue.textContent, "15");
  assert.equal(controls.crfValue.textContent, "40");
  assert.equal(controls.scaleValue.textContent, "3.5");
  assert.equal(controls.scrollValue.textContent, "2.25×");
});

test("a rejected save stays visible until dismissed and can be retried", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let fail = true;
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31, maxScale: 2, scrollSensitivity: 1 }),
    save: async (settings) => {
      if (fail) throw new Error("could not save the settings");
      return settings;
    },
  });
  await screen.open();
  click(controls.saveButton);
  await settle();
  assert.equal(controls.statusElement.textContent, "could not save the settings");
  assert.equal(controls.noticeElement.hidden, false);
  t.mock.timers.tick(10000);
  assert.equal(controls.noticeElement.hidden, false);
  click(controls.dismissButton);
  assert.equal(controls.noticeElement.hidden, true);
  assert.equal(controls.saveButton.disabled, false);
  fail = false;
  click(controls.saveButton);
  await settle();
  assert.match(controls.statusElement.textContent, /^Saved/);
  assert.equal(controls.statusElement.classes.has("has-error"), false);
});

test("saving again restarts the notice timeout", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31, maxScale: 2, scrollSensitivity: 1 }),
    save: async (settings) => settings,
  });
  await screen.open();
  click(controls.saveButton);
  await settle();
  t.mock.timers.tick(4000);
  click(controls.saveButton);
  await settle();
  t.mock.timers.tick(1000);
  assert.equal(controls.noticeElement.hidden, false);
  t.mock.timers.tick(4000);
  assert.equal(controls.noticeElement.hidden, true);
});

test("leaving settings clears the notice and suppresses a pending save notice", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let finishSave;
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31, maxScale: 2, scrollSensitivity: 1 }),
    save: (settings) => new Promise(resolve => { finishSave = () => resolve(settings); }),
  });
  await screen.open();
  click(controls.saveButton);
  screen.close();
  finishSave();
  await settle();
  assert.equal(controls.noticeElement.hidden, true);
  assert.equal(controls.statusElement.textContent, "");
  await screen.open();
  click(controls.saveButton);
  finishSave();
  await settle();
  assert.equal(controls.noticeElement.hidden, false);
  screen.close();
  assert.equal(controls.noticeElement.hidden, true);
  t.mock.timers.tick(5000);
  await screen.open();
  assert.equal(controls.noticeElement.hidden, true);
});
