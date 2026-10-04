import test from "node:test";
import assert from "node:assert/strict";
import { setImmediate } from "node:timers/promises";
import { createRemoteConnection } from "../src/core/webrtc.js";
import { renderInputMode } from "../src/ui/input-mode.js";

function setup(t) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let peer, socket;
  const sent = [];
  class Channel extends EventTarget {
    readyState = "open";
    bufferedAmount = 0;
    send(data) { sent.push(JSON.parse(data)); }
    close() { this.readyState = "closed"; }
  }
  class Peer {
    connectionState = "connected";
    constructor() { peer = this; }
    createDataChannel() { return this.channel = new Channel(); }
    addTransceiver() {}
    close() { this.connectionState = "closed"; }
  }
  class Socket extends EventTarget {
    static OPEN = 1;
    readyState = 1;
    constructor() { super(); socket = this; }
    send() {}
    close() { this.readyState = 3; }
  }
  globalThis.window = { location: { protocol: "https:", host: "test" }, setTimeout, clearTimeout };
  globalThis.RTCPeerConnection = Peer;
  globalThis.WebSocket = Socket;
  const video = Object.assign(new EventTarget(), { play: () => Promise.resolve() });
  const attrs = {};
  const children = Object.fromEntries([".input-window-icon", ".input-pc-icon"].map(k => [k, { toggleAttribute(name, value) { this[name] = value; } }]));
  const button = { setAttribute: (k,v) => attrs[k] = v, querySelector: (k) => children[k] };
  let failure;
  const ready = createRemoteConnection({
    videoElement: video,
    onInputMode: (mode,switching) => renderInputMode(button,mode,switching,true),
    onDisconnect: (error) => failure = error,
  });
  peer.ontrack({ streams: [{ getTracks: () => [] }] });
  video.dispatchEvent(new Event("loadeddata"));
  return { ready, sent, attrs, button, children, peer, get failure() { return failure; },
    async confirm(mode) { socket.dispatchEvent(Object.assign(new Event("message"), { data: JSON.stringify({ type: "input.mode", mode }) })); await setImmediate(); },
  };
}

test("mode requests follow pending input, block sends and wait for host confirmation", async (t) => {
  const f = setup(t), remote = await f.ready;
  t.after(() => remote.close());
  assert.equal(f.attrs["aria-pressed"], "false");
  remote.sendControl({ type: "input.mouseMove", x: .1, y: .1 });
  remote.sendControl({ type: "input.mouseMove", x: .2, y: .2 });
  assert.equal(f.sent.length, 1);
  assert.equal(remote.setInputMode("pc"), true);
  assert.deepEqual(f.sent.map(c => c.type), ["input.mouseMove", "input.mouseMove", "input.mode"]);
  assert.equal(remote.sendControl({ type: "input.text", text: "blocked" }), false);
  assert.equal(remote.setInputMode("window"), false);
  assert.equal(remote.inputMode, "window");
  assert.equal(f.attrs["aria-pressed"], "false");
  assert.equal(f.button.disabled, true);
  assert.equal(f.attrs["aria-busy"], "true");
  await f.confirm("pc");
  assert.equal(remote.inputMode, "pc");
  assert.equal(f.attrs["aria-pressed"], "true");
  assert.equal(f.attrs["aria-label"], "PC control: on. Switch to Window mode");
  assert.equal(f.button.disabled, false);
  assert.equal(f.attrs["aria-busy"], "false");
  assert.equal(f.children[".input-window-icon"].hidden, true);
  assert.equal(f.children[".input-pc-icon"].hidden, false);
  assert.equal(remote.sendControl({ type: "input.keyUp", key: "Enter" }), true);
  t.mock.timers.tick(15000);
  assert.equal(f.failure, undefined);
});

test("missing mode confirmation ends connection after 15 seconds", async (t) => {
  const f = setup(t), remote = await f.ready;
  remote.setInputMode("pc");
  t.mock.timers.tick(14999);
  assert.equal(f.failure, undefined);
  t.mock.timers.tick(1);
  assert.match(f.failure.message, /switch timed out/);
  assert.equal(remote.sendControl({ type: "input.tap" }), false);
});

test("mismatched mode confirmation is a connection failure", async (t) => {
  const f = setup(t), remote = await f.ready;
  remote.setInputMode("pc");
  await f.confirm("window");
  assert.match(f.failure.message, /Invalid input mode confirmation/);
});
