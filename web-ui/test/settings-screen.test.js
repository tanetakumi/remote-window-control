import test from "node:test";
import assert from "node:assert/strict";
import { attachSettingsScreen } from "../src/ui/settings-screen.js";

class Element extends EventTarget {
  value = "";
  textContent = "";
  disabled = false;
  attributes = {};
  classes = new Set();
  classList = { toggle: (name, on) => (on ? this.classes.add(name) : this.classes.delete(name)) };
  setAttribute(name, value) { this.attributes[name] = value; }
}

function createScreen(api) {
  const controls = {
    fpsInput: new Element(), fpsValue: new Element(),
    crfInput: new Element(), crfValue: new Element(),
    saveButton: new Element(), statusElement: new Element(),
  };
  return { controls, screen: attachSettingsScreen(controls, api) };
}

const click = (element) => element.dispatchEvent(new Event("click"));
const settle = () => new Promise((resolve) => setImmediate(resolve));

test("opening loads the stored settings and enables Save", async () => {
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 12, crf: 24 }),
    save: async () => assert.fail("not saved"),
  });
  assert.equal(controls.saveButton.disabled, true);
  await screen.open();
  assert.equal(controls.fpsInput.value, "12");
  assert.equal(controls.crfInput.value, "24");
  assert.equal(controls.saveButton.disabled, false);
  assert.equal(controls.statusElement.textContent, "");
});

test("a failed load reports the error and keeps Save disabled", async () => {
  const { controls, screen } = createScreen({
    load: async () => { throw new Error("host unreachable"); },
    save: async () => assert.fail("not saved"),
  });
  await screen.open();
  assert.equal(controls.statusElement.textContent, "host unreachable");
  assert.equal(controls.statusElement.classes.has("has-error"), true);
  assert.equal(controls.saveButton.disabled, true);
  click(controls.saveButton);
});

test("Save sends the edited values and shows what the host stored", async () => {
  const saved = [];
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31 }),
    save: async (settings) => { saved.push(settings); return settings; },
  });
  await screen.open();
  controls.fpsInput.value = "20";
  controls.crfInput.value = "0";
  controls.fpsInput.dispatchEvent(new Event("input"));
  click(controls.saveButton);
  assert.equal(controls.saveButton.disabled, true);
  click(controls.saveButton);
  await settle();
  assert.deepEqual(saved, [{ fps: 20, crf: 0 }]);
  assert.match(controls.statusElement.textContent, /^Saved/);
  assert.equal(controls.statusElement.classes.has("has-error"), false);
  assert.equal(controls.saveButton.disabled, false);
  assert.equal(controls.fpsValue.textContent, "20");
});

test("the sliders show their value while moving", async () => {
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31 }),
    save: async () => assert.fail("not saved"),
  });
  await screen.open();
  assert.equal(controls.fpsValue.textContent, "8");
  assert.equal(controls.crfValue.textContent, "31");
  controls.fpsInput.value = "15";
  controls.fpsInput.dispatchEvent(new Event("input"));
  controls.crfInput.value = "40";
  controls.crfInput.dispatchEvent(new Event("input"));
  assert.equal(controls.fpsValue.textContent, "15");
  assert.equal(controls.crfValue.textContent, "40");
});

test("a rejected save shows the host message and can be retried", async () => {
  let fail = true;
  const { controls, screen } = createScreen({
    load: async () => ({ fps: 8, crf: 31 }),
    save: async (settings) => {
      if (fail) throw new Error("could not save the settings");
      return settings;
    },
  });
  await screen.open();
  click(controls.saveButton);
  await settle();
  assert.equal(controls.statusElement.textContent, "could not save the settings");
  assert.equal(controls.saveButton.disabled, false);
  fail = false;
  click(controls.saveButton);
  await settle();
  assert.match(controls.statusElement.textContent, /^Saved/);
});
