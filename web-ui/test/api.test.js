import test from "node:test";
import assert from "node:assert/strict";
import { fetchWindows, setTargetWindow, fetchKeepalive, setKeepalive } from "../src/core/api.js";

test("window APIs work without application credentials", async (t) => {
  const target = { handle: 123, title: "Test window" };
  const keepalive = { state: "connected", enabled: true };
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url, options) => {
    calls.push({ url, options });
    if (url === "/api/windows") return Response.json([target]);
    if (url === "/api/session-keepalive") return Response.json(keepalive);
    return Response.json(target);
  });

  assert.deepEqual(await fetchWindows(), [target]);
  assert.deepEqual(await setTargetWindow(target.handle), target);
  assert.deepEqual(await fetchKeepalive(), keepalive);
  assert.deepEqual(await setKeepalive(false), keepalive);
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
    { url: "/api/session-keepalive", options: undefined },
    {
      url: "/api/session-keepalive",
      options: {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: false }),
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
  await assert.rejects(fetchKeepalive(), /Request rejected/);
  await assert.rejects(setKeepalive(true), /Request rejected/);
  assert.deepEqual(calls, ["/api/windows", "/api/target-window", "/api/session-keepalive", "/api/session-keepalive"]);
});

test("window loading preserves helper diagnostics and falls back on empty errors", async (t) => {
  t.mock.method(globalThis, "fetch", async () =>
    new Response("  You must install .NET to run this application.\n", { status: 502 }),
  );
  await assert.rejects(fetchWindows(), /You must install \.NET to run this application\./);

  t.mock.method(globalThis, "fetch", async () => new Response("", { status: 502 }));
  await assert.rejects(fetchWindows(), /Could not load windows/);
});
