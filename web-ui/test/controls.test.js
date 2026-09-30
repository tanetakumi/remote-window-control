import test from "node:test";
import assert from "node:assert/strict";
import { attachGestureControls } from "../src/gestures.js";
import { attachKeyboardBridge } from "../src/keyboard.js";
import { createRemoteConnection } from "../src/webrtc.js";

class Element extends EventTarget {
  value = "";
  srcObject = null;
  classList = { toggle() {} };
  focus() {}
  blur() {}
  setSelectionRange() {}
  getBoundingClientRect() { return { left: 0, top: 0, width: 100, height: 100 }; }
  play() { return Promise.resolve(); }
}
function emit(target, type, fields = {}) {
  const event = new Event(type, { cancelable: true });
  Object.assign(event, fields);
  target.dispatchEvent(event);
}
globalThis.window = globalThis;
window.location = { protocol: "http:", host: "host:8443" };
const finger = (x, y) => ({ clientX: x, clientY: y });

test("two fingers ending separately never generate a tap; disposed listeners send nothing", () => {
  const video = new Element();
  const sent = [];
  const dispose = attachGestureControls(video, (value) => sent.push(value), () => {});
  emit(video, "touchstart", { touches: [finger(10, 10)], changedTouches: [] });
  emit(video, "touchstart", { touches: [finger(10, 10), finger(30, 10)], changedTouches: [] });
  emit(video, "touchend", { touches: [finger(30, 10)], changedTouches: [finger(10, 10)] });
  emit(video, "touchend", { touches: [], changedTouches: [finger(30, 10)] });
  assert.deepEqual(sent, []);
  dispose();
  emit(video, "touchstart", { touches: [finger(10, 10)], changedTouches: [] });
  emit(video, "touchend", { touches: [], changedTouches: [finger(10, 10)] });
  assert.deepEqual(sent, []);
});

test("touch cancellation releases a right-button hold", async () => {
  const video = new Element();
  const sent = [];
  const dispose = attachGestureControls(video, (value) => sent.push(value), () => {});
  emit(video, "touchstart", { touches: [finger(10, 10)], changedTouches: [] });
  await new Promise((resolve) => setTimeout(resolve, 480));
  emit(video, "touchcancel", { touches: [], changedTouches: [finger(10, 10)] });
  assert.deepEqual(sent.map((c) => c.type), ["input.mouseDown", "input.mouseUp"]);
  dispose();
});

test("space has one text path; IME only commits once; replacements erase the old suffix", () => {
  const controls = { buttonElement: new Element(), inputElement: new Element(), backspaceButton: new Element(), enterButton: new Element() };
  const sent = [];
  const bridge = attachKeyboardBridge(controls, (c) => sent.push(c));
  emit(controls.buttonElement, "click");
  emit(controls.inputElement, "keydown", { key: " " });
  controls.inputElement.value = " ";
  emit(controls.inputElement, "input");
  emit(controls.inputElement, "keyup", { key: " " });
  assert.deepEqual(sent, [{ type: "input.text", text: " " }]);
  emit(controls.inputElement, "compositionstart");
  controls.inputElement.value = " に";
  emit(controls.inputElement, "input", { isComposing: true });
  emit(controls.inputElement, "keydown", { key: "Enter", isComposing: true });
  assert.equal(sent.length, 1);
  controls.inputElement.value = " 日本";
  emit(controls.inputElement, "compositionend");
  emit(controls.inputElement, "input");
  assert.deepEqual(sent[1], { type: "input.text", text: "日本" });
  controls.inputElement.value = " 日々";
  emit(controls.inputElement, "input", { inputType: "insertReplacementText" });
  assert.deepEqual(sent.slice(2), [{ type: "input.keyDown", key: "Backspace" }, { type: "input.keyUp", key: "Backspace" }, { type: "input.text", text: "々" }]);
  bridge.cleanup();
  emit(controls.enterButton, "click");
  assert.equal(sent.length, 5);
});

test("large pasted text is sent in bounded Unicode-safe commands", () => {
  const controls = { buttonElement: new Element(), inputElement: new Element() };
  const sent = [];
  const bridge = attachKeyboardBridge(controls, (c) => sent.push(c));
  emit(controls.inputElement, "focus");
  controls.inputElement.value = "😀".repeat(2500);
  emit(controls.inputElement, "input", { inputType: "insertFromPaste" });
  assert.equal(sent.length, 3);
  assert.equal(sent.map((c) => c.text).join(""), controls.inputElement.value);
  sent.forEach((c) => assert.ok(Buffer.byteLength(c.text) <= 4096));
  bridge.cleanup();
});

class Channel extends EventTarget {
  readyState = "open";
  bufferedAmount = 0;
  send() {}
  close() { this.readyState = "closed"; }
}
class Peer {
  static current;
  connectionState = "new";
  remoteDescription = null;
  candidates = [];
  constructor() { Peer.current = this; }
  createDataChannel() { return new Channel(); }
  addTransceiver() {}
  async createOffer() { return { sdp: "offer" }; }
  async setLocalDescription() {}
  async setRemoteDescription(value) { this.remoteDescription = value; }
  async addIceCandidate(value) { assert.ok(this.remoteDescription); this.candidates.push(value); }
  close() { this.connectionState = "closed"; }
}
class Socket extends EventTarget {
  static OPEN = 1;
  static current;
  readyState = 1;
  constructor() { super(); Socket.current = this; }
  send() {}
  close() { this.readyState = 3; }
}
globalThis.RTCPeerConnection = Peer;
globalThis.WebSocket = Socket;
const flush = async () => { for (let i = 0; i < 6; i++) await Promise.resolve(); };

test("connection waits for media and decoded video, queues early ICE, and closes on failure", async () => {
  const video = new Element();
  let resolved = false;
  let stopped = false;
  let failure;
  const pending = createRemoteConnection({ token: "test", videoElement: video, onDisconnect: (error) => { failure = error; } });
  pending.then(() => { resolved = true; });
  emit(Socket.current, "open");
  emit(Socket.current, "message", { data: JSON.stringify({ type: "webrtc.ice", candidate: { candidate: "early" } }) });
  emit(Socket.current, "message", { data: JSON.stringify({ type: "webrtc.answer", sdp: "answer" }) });
  await flush();
  assert.equal(Peer.current.candidates.length, 1);
  assert.equal(resolved, false);
  Peer.current.ontrack({ streams: [{ getTracks: () => [{ stop: () => { stopped = true; } }] }] });
  Peer.current.connectionState = "connected";
  Peer.current.onconnectionstatechange();
  await flush();
  assert.equal(resolved, false);
  emit(video, "loadeddata");
  const remote = await pending;
  assert.equal(resolved, true);
  emit(Socket.current, "message", { data: JSON.stringify({ type: "error", message: "host failure" }) });
  await flush();
  assert.equal(failure.message, "host failure");
  assert.equal(stopped, true);
  assert.equal(video.srcObject, null);
  assert.equal(remote.sendControl({ type: "input.tap" }), false);
});

test("host error rejects readiness and abort cancels a pending connection", async () => {
  const video = new Element();
  const pending = createRemoteConnection({ token: "test", videoElement: video });
  const rejected = assert.rejects(pending, /busy/);
  emit(Socket.current, "message", { data: JSON.stringify({ type: "error", message: "busy" }) });
  await rejected;
  assert.equal(Peer.current.connectionState, "closed");
  const abort = new AbortController();
  const cancelled = createRemoteConnection({ token: "test", videoElement: video, signal: abort.signal });
  const rejection = assert.rejects(cancelled, /cancelled/);
  abort.abort();
  await rejection;
});
