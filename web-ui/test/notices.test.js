import test from "node:test";
import assert from "node:assert/strict";
import { createNotices } from "../src/ui/notices.js";

test("connection recovery clears transient status without erasing an input error", () => {
  let text;
  const notices = createNotices(message => text = message);
  notices.transient("Reconnecting…");
  assert.equal(text, "Reconnecting…");
  notices.transient("");
  assert.equal(text, "");
  notices.transient("Input delayed…");
  notices.show("Other window obscures the target");
  notices.transient("");
  assert.equal(text, "Other window obscures the target");
  notices.dismiss();
  assert.equal(text, "");
  notices.show("error");
  notices.reset();
  assert.equal(text, "");
});
