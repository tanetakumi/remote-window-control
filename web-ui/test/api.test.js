import test from "node:test";
import assert from "node:assert/strict";
import { fetchWindows, setTargetWindow } from "../src/core/api.js";

test("window APIs work without application credentials", async (t) => {
  const target = { handle: 123, title: "Test window" };
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url, options) => {
    calls.push({ url, options });
    return Response.json(url === "/api/windows" ? [target] : target);
  });

  assert.deepEqual(await fetchWindows(), [target]);
  assert.deepEqual(await setTargetWindow(target.handle), target);
  assert.deepEqual(calls, [
    { url: "/api/windows", options: undefined },
    {
      url: "/api/target-window",
      options: {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ handle: target.handle }),
      },
    },
  ]);
});

test("rejected API requests report errors without attempting authentication", async (t) => {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url) => {
    calls.push(url);
    return new Response("Request rejected", { status: 401 });
  });

  await assert.rejects(fetchWindows(), /Request rejected/);
  await assert.rejects(setTargetWindow(123), /Request rejected/);
  assert.deepEqual(calls, ["/api/windows", "/api/target-window"]);
});

test("window loading preserves helper diagnostics and falls back on empty errors", async (t) => {
  t.mock.method(globalThis, "fetch", async () =>
    new Response("  You must install .NET to run this application.\n", { status: 502 }),
  );
  await assert.rejects(fetchWindows(), /You must install \.NET to run this application\./);

  t.mock.method(globalThis, "fetch", async () => new Response("", { status: 502 }));
  await assert.rejects(fetchWindows(), /Could not load windows/);
});
