import test, { mock } from "node:test";
import assert from "node:assert/strict";
import { attachTextInput } from "../src/input/keyboard.js";
import { createRemoteConnection } from "../src/core/webrtc.js";

class Element extends EventTarget {
  value = "";
  srcObject = null;
  classList = { toggle() {} };
  style = { setProperty() {} };
  attributes = {};
  disabled = false;
  hidden = false;
  open = false;
  focusCount = 0;
  focus() { this.focusCount++; }
  blur() {}
  setAttribute(name, value) { this.attributes[name] = value; }
  showModal() { this.open = true; }
  close() { this.open = false; emit(this, "close"); }
  setSelectionRange() {}
  getBoundingClientRect() { return { left: 0, top: 0, width: 100, height: 100, right: 100, bottom: 100 }; }
  play() { return Promise.resolve(); }
}
function emit(target, type, fields = {}) {
  const event = new Event(type, { cancelable: true });
  Object.assign(event, fields);
  target.dispatchEvent(event);
}
globalThis.window = Object.assign(new EventTarget(), {
  setTimeout: (...args) => setTimeout(...args),
  clearTimeout: (...args) => clearTimeout(...args),
  innerWidth: 390, innerHeight: 844,
});
window.location = { protocol: "http:", host: "host:8443" };

function createEditor(draft = { text: "", lastSent: "", error: "" }, sendControl) {
  const controls = {
    buttonElement: new Element(), dialogElement: new Element(), inputElement: new Element(),
    closeButton: new Element(), sendButton: new Element(), sendEnterButton: new Element(),
    restoreButton: new Element(),
    errorElement: new Element(), draft,
  };
  const sent = [];
  const editor = attachTextInput(controls, sendControl ?? ((command) => { sent.push(command); return true; }));
  return { controls, sent, editor };
}

function edit(controls, value, fields) {
  controls.inputElement.value = value;
  emit(controls.inputElement, "input", fields);
}

test("typing, IME, replacement, deletion and special keys stay local until Send", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.buttonElement, "click");
  assert.equal(controls.dialogElement.open, true);
  assert.equal(controls.inputElement.focusCount, 1);
  assert.equal(controls.buttonElement.attributes["aria-expanded"], "true");
  assert.equal(controls.sendButton.disabled, true);
  edit(controls, " ");
  emit(controls.inputElement, "compositionstart");
  edit(controls, " に", { isComposing: true });
  emit(controls.inputElement, "keydown", { key: "Enter", isComposing: true });
  emit(controls.sendButton, "click");
  assert.equal(controls.sendButton.disabled, true);
  edit(controls, " 日本");
  emit(controls.inputElement, "compositionend");
  edit(controls, " 日々", { inputType: "insertReplacementText" });
  for (const key of ["Enter", "Backspace", "Tab", "ArrowLeft", "Delete"]) {
    emit(controls.inputElement, "keydown", { key });
  }
  edit(controls, " 日", { inputType: "deleteContentBackward" });
  assert.deepEqual(sent, []);
  emit(controls.sendButton, "click");
  emit(controls.sendButton, "click");
  assert.deepEqual(sent, [{ type: "input.text", text: " 日" }]);
  assert.equal(controls.inputElement.value, "");
  assert.equal(controls.draft.text, "");
  assert.equal(controls.dialogElement.open, false);
  assert.equal(controls.buttonElement.attributes["aria-expanded"], "false");
  editor.cleanup();
});

test("empty drafts cannot send, but whitespace is preserved", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.sendButton, "click");
  emit(controls.buttonElement, "click");
  emit(controls.sendButton, "click");
  assert.deepEqual(sent, []);
  edit(controls, "  ");
  emit(controls.sendButton, "click");
  assert.deepEqual(sent, [{ type: "input.text", text: "  " }]);
  editor.cleanup();
});

test("the whole text, line breaks included, is sent as one command on demand", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.buttonElement, "click");
  const text = "日本\n\n😀".repeat(500) + "world";
  edit(controls, text, { inputType: "insertFromPaste" });
  assert.deepEqual(sent, []);
  emit(controls.sendButton, "click");
  assert.deepEqual(sent, [{ type: "input.text", text }]);
  editor.cleanup();
});

test("Send + Enter asks the host to press Enter after the same paste", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.buttonElement, "click");
  assert.equal(controls.sendEnterButton.disabled, true);
  edit(controls, "ls -la\n");
  assert.equal(controls.sendEnterButton.disabled, false);
  emit(controls.inputElement, "compositionstart");
  emit(controls.sendEnterButton, "click");
  assert.equal(controls.sendEnterButton.disabled, true);
  emit(controls.inputElement, "compositionend");
  emit(controls.sendEnterButton, "click");
  emit(controls.sendEnterButton, "click");
  assert.deepEqual(sent, [{ type: "input.text", text: "ls -la\n", enter: true }]);
  assert.equal(controls.dialogElement.open, false);
  editor.cleanup();
});

test("touch presses on dialog buttons do not steal focus from the textarea", () => {
  const { controls, editor } = createEditor();
  emit(controls.buttonElement, "click");
  for (const name of ["sendButton", "sendEnterButton", "restoreButton", "closeButton"]) {
    const event = new Event("pointerdown", { cancelable: true });
    Object.assign(event, { pointerType: "touch", isPrimary: true, button: 0 });
    controls[name].dispatchEvent(event);
    assert.equal(event.defaultPrevented, true, name);
  }
  editor.cleanup();
});

test("closing and reconnecting preserve the draft and cleanup removes old listeners", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.buttonElement, "click");
  edit(controls, "unfinished draft");
  emit(controls.closeButton, "click");
  assert.equal(editor.isActive(), false);
  emit(controls.buttonElement, "click");
  assert.equal(controls.inputElement.value, "unfinished draft");
  editor.cleanup();
  assert.equal(controls.dialogElement.open, false);
  assert.equal(controls.buttonElement.disabled, true);
  emit(controls.buttonElement, "click");
  emit(controls.sendButton, "click");
  assert.deepEqual(sent, []);
  const reconnected = createEditor(controls.draft);
  emit(reconnected.controls.buttonElement, "click");
  assert.equal(reconnected.controls.inputElement.value, "unfinished draft");
  reconnected.editor.cleanup();
});

test("close touch keeps input focused until click; cancellation retains the draft", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.buttonElement, "click");
  edit(controls, "unfinished draft");
  const touchDown = () => {
    const event = new Event("pointerdown", { cancelable: true });
    Object.assign(event, { pointerType: "touch", isPrimary: true, button: 0 });
    controls.closeButton.dispatchEvent(event);
    return event;
  };
  assert.equal(touchDown().defaultPrevented, true, "suppress focus transfer that dismisses the keyboard");
  assert.equal(editor.isActive(), true, "pressing down does not close the editor");
  emit(controls.closeButton, "pointercancel");
  assert.equal(editor.isActive(), true);
  assert.equal(controls.draft.text, "unfinished draft");
  assert.equal(touchDown().defaultPrevented, true);
  emit(controls.closeButton, "pointerup");
  emit(controls.closeButton, "click");
  assert.equal(editor.isActive(), false);
  assert.equal(controls.draft.text, "unfinished draft");
  assert.deepEqual(sent, []);
  editor.cleanup();
  assert.equal(touchDown().defaultPrevented, false, "cleanup removes the touch handler");
});

test("close focus guard leaves mouse and secondary touches unchanged", () => {
  const { controls, editor } = createEditor();
  emit(controls.buttonElement, "click");
  for (const fields of [
    { pointerType: "mouse", isPrimary: true, button: 0 },
    { pointerType: "touch", isPrimary: false, button: 0 },
    { pointerType: "touch", isPrimary: true, button: 2 },
  ]) {
    const event = new Event("pointerdown", { cancelable: true });
    Object.assign(event, fields);
    controls.closeButton.dispatchEvent(event);
    assert.equal(event.defaultPrevented, false);
  }
  emit(controls.closeButton, "click", { detail: 0 });
  assert.equal(editor.isActive(), false, "keyboard activation still closes via click");
  editor.cleanup();
});

test("Escape during composition is ignored; backdrop dismisses only an outside press and click", () => {
  const { controls, sent, editor } = createEditor();
  emit(controls.buttonElement, "click");
  emit(controls.inputElement, "compositionstart");
  const cancel = new Event("cancel", { cancelable: true });
  controls.dialogElement.dispatchEvent(cancel);
  assert.equal(cancel.defaultPrevented, true);
  emit(controls.inputElement, "compositionend");
  const escape = new Event("cancel", { cancelable: true });
  controls.dialogElement.dispatchEvent(escape);
  assert.equal(escape.defaultPrevented, false);
  emit(controls.dialogElement, "pointerdown", { clientX: 50, clientY: 50 });
  emit(controls.dialogElement, "click", { clientX: 150, clientY: 150 });
  assert.equal(editor.isActive(), true, "dragging out of the panel does not dismiss it");
  emit(controls.dialogElement, "pointerdown", { clientX: 150, clientY: 150 });
  emit(controls.dialogElement, "click", { clientX: 150, clientY: 150 });
  assert.equal(editor.isActive(), false);
  assert.deepEqual(sent, []);
  editor.cleanup();
});

test("a failed send retains the text and does not retry automatically", () => {
  const commands = [];
  const { controls, editor } = createEditor(undefined, (command) => {
    commands.push(command);
    return false;
  });
  emit(controls.buttonElement, "click");
  const text = "😀".repeat(2500);
  edit(controls, text);
  emit(controls.sendButton, "click");
  assert.equal(commands.length, 1);
  assert.equal(controls.draft.text, text);
  assert.equal(controls.inputElement.value, text);
  assert.equal(controls.errorElement.hidden, false);
  assert.match(controls.errorElement.textContent, /could not be sent/);
  assert.equal(editor.isActive(), true);
  editor.cleanup();
});

test("synchronous disconnect during sending keeps the draft", () => {
  const commands = [];
  let disconnect;
  const { controls, editor } = createEditor(undefined, (command) => {
    commands.push(command);
    disconnect();
    return false;
  });
  disconnect = () => editor.cleanup();
  emit(controls.buttonElement, "click");
  edit(controls, "😀".repeat(2500));
  emit(controls.sendButton, "click");
  assert.equal(commands.length, 1);
  assert.equal(controls.draft.text, "😀".repeat(2500));
  assert.equal(editor.isActive(), false);
  assert.equal(controls.buttonElement.disabled, true);
  assert.match(controls.draft.error, /could not be sent/);
});

test("a later host error offers the last text without replacing a new draft", () => {
  const { controls, editor } = createEditor();
  emit(controls.buttonElement, "click");
  edit(controls, "submitted");
  emit(controls.sendButton, "click");
  editor.reportError("Host input timed out");
  emit(controls.buttonElement, "click");
  assert.equal(controls.restoreButton.hidden, false);
  emit(controls.restoreButton, "click");
  assert.equal(controls.inputElement.value, "submitted");
  edit(controls, "new draft");
  editor.reportError("Another host error");
  assert.equal(controls.restoreButton.hidden, true);
  assert.equal(controls.inputElement.value, "new draft");
  editor.cleanup();
});

class Channel extends EventTarget {
  static current;
  readyState = "connecting";
  bufferedAmount = 0;
  sent = [];
  constructor() { super(); Channel.current = this; }
  open() { this.readyState = "open"; emit(this, "open"); }
  send(data) { this.sent.push(data); }
  close() { this.readyState = "closed"; }
}
class Peer {
  static current;
  connectionState = "new";
  remoteDescription = null;
  candidates = [];
  constructor() { Peer.current = this; }
  transceivers = [];
  createDataChannel(label) { return Object.assign(new Channel(), { label }); }
  addTransceiver(kind) { this.transceivers.push(kind); }
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
  constructor(url) { super(); this.url = url; Socket.current = this; }
  send() {}
  close() { this.readyState = 3; }
}
globalThis.RTCPeerConnection = Peer;
globalThis.WebSocket = Socket;
const flush = async () => { for (let i = 0; i < 6; i++) await Promise.resolve(); };
// The host's first message carries its settings; the offer does not wait for it.
function configure(fields = {}) {
  emit(Socket.current, "open");
  emit(Socket.current, "message", { data: JSON.stringify({ type: "session.config", ...fields }) });
}

test("connection waits for media and decoded video, queues early ICE, and closes on failure", async () => {
  const video = new Element();
  let resolved = false;
  let stopped = false;
  let failure;
  const statuses = [], inputErrors = [];
  const pending = createRemoteConnection({
    videoElement: video,
    onDisconnect: (error) => { failure = error; },
    onStatus: (state, message) => statuses.push({ state, message }),
    onInputError: (message) => inputErrors.push(message),
  });
  assert.equal(Socket.current.url, "ws://host:8443/ws");
  pending.then(() => { resolved = true; });
  configure();
  await flush();
  assert.deepEqual(statuses.at(-1), { state: "connecting", message: "Waiting for video…" });
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
  await flush();
  assert.equal(resolved, false, "not ready until the control channel is open");
  Channel.current.open();
  const remote = await pending;
  assert.equal(resolved, true);
  assert.deepEqual(statuses.at(-1), { state: "connected", message: "Connected" });
  emit(Socket.current, "message", { data: JSON.stringify({ type: "input.error", message: "text input timed out" }) });
  await flush();
  assert.deepEqual(inputErrors, ["text input timed out"]);
  assert.deepEqual(statuses.at(-1), { state: "connected", message: "Connected" });
  emit(Socket.current, "message", { data: JSON.stringify({ type: "error", message: "host failure" }) });
  await flush();
  assert.equal(failure.message, "host failure");
  assert.deepEqual(statuses.at(-1), { state: "error", message: "Connection lost" });
  assert.equal(stopped, true);
  assert.equal(video.srcObject, null);
  assert.equal(remote.sendControl({ type: "input.tap" }), false);
});

test("host error rejects readiness and abort cancels a pending connection", async () => {
  const video = new Element();
  window.location = { protocol: "https:", host: "remote.example.com" };
  const pending = createRemoteConnection({ videoElement: video });
  assert.equal(Socket.current.url, "wss://remote.example.com/ws");
  const rejected = assert.rejects(pending, /busy/);
  emit(Socket.current, "message", { data: JSON.stringify({ type: "error", message: "busy" }) });
  await rejected;
  assert.equal(Peer.current.connectionState, "closed");
  const abort = new AbortController();
  const cancelled = createRemoteConnection({ videoElement: video, signal: abort.signal });
  const rejection = assert.rejects(cancelled, /cancelled/);
  abort.abort();
  await rejection;
});

test("input is sent only on the control data channel, never on the signaling socket", async () => {
  const video = new Element();
  const pending = createRemoteConnection({ videoElement: video });
  const socketSent = [];
  Socket.current.send = (data) => socketSent.push(data);
  configure();
  emit(Socket.current, "message", { data: JSON.stringify({ type: "webrtc.answer", sdp: "answer" }) });
  await flush();
  Peer.current.ontrack({ streams: [{ getTracks: () => [] }] });
  Peer.current.connectionState = "connected";
  Peer.current.onconnectionstatechange();
  emit(video, "loadeddata");
  Channel.current.open();
  const remote = await pending;
  const before = socketSent.length;

  assert.equal(remote.sendControl({ type: "input.tap", x: 0.5 }), true);
  assert.deepEqual(Channel.current.sent.map((s) => JSON.parse(s)), [{ type: "input.tap", x: 0.5 }]);
  assert.equal(socketSent.length, before, "nothing is sent on the WebSocket");
  remote.close();
});

test("a control channel that never opens fails the connection instead of falling back", async () => {
  const video = new Element();
  const pending = createRemoteConnection({ videoElement: video });
  const rejected = assert.rejects(pending, /Control channel closed/);
  configure();
  emit(Socket.current, "message", { data: JSON.stringify({ type: "webrtc.answer", sdp: "answer" }) });
  await flush();
  Peer.current.ontrack({ streams: [{ getTracks: () => [] }] });
  Peer.current.connectionState = "connected";
  Peer.current.onconnectionstatechange();
  emit(video, "loadeddata");
  await flush();
  emit(Channel.current, "close");
  await rejected;
});

test("initial viewport is resized when control opens before the first video frame", async () => {
  const video = new Element();
  let resolved = false;
  const pending = createRemoteConnection({
    videoElement: video,
    getInitialViewport: () => ({ width: 390, height: 720, devicePixelRatio: 2, type: "input.text", text: "not sent" }),
  });
  pending.then(() => { resolved = true; });
  configure();
  Peer.current.connectionState = "connected";
  Peer.current.onconnectionstatechange();
  Channel.current.open();
  await flush();
  assert.equal(resolved, false, "resize does not mark video ready");
  assert.deepEqual(Channel.current.sent.map(JSON.parse), [{ type: "viewport.resize", width: 390, height: 720, devicePixelRatio: 2 }]);
  Peer.current.ontrack({ streams: [{ getTracks: () => [] }] });
  emit(video, "loadeddata");
  const remote = await pending;
  remote.close();
});

test("a playback prompt is a notice and never marks the control connection ready", async () => {
  const video = new Element();
  video.play = () => Promise.reject(new Error("Autoplay blocked"));
  const statuses = [], notices = [];
  const pending = createRemoteConnection({
    videoElement: video,
    onStatus: (state, message) => statuses.push({ state, message }),
    onNotice: (message) => notices.push(message),
  });
  configure();
  await flush();
  Peer.current.ontrack({ streams: [{ getTracks: () => [] }] });
  await flush();
  assert.deepEqual(notices, ["Tap the video to start playback."]);
  assert.deepEqual(statuses.at(-1), { state: "connecting", message: "Waiting for video…" });
  const rejected = assert.rejects(pending, /cancelled/);
  emit(Socket.current, "message", { data: JSON.stringify({ type: "error", message: "cancelled" }) });
  await rejected;
});

test("a brief disconnection is waited out on both sides of the grace period", async (t) => {
  mock.timers.enable({ apis: ["setTimeout"] });
  t.after(() => mock.timers.reset());
  const video = new Element();
  const statuses = [];
  let failure;
  const pending = createRemoteConnection({
    videoElement: video,
    onStatus: (state, message) => statuses.push({ state, message }),
    onDisconnect: (error) => { failure = error; },
  });
  configure();
  await flush();
  Peer.current.ontrack({ streams: [{ getTracks: () => [] }] });
  Peer.current.connectionState = "connected";
  Peer.current.onconnectionstatechange();
  emit(video, "loadeddata");
  Channel.current.open();
  const remote = await pending;

  const setState = (state) => { Peer.current.connectionState = state; Peer.current.onconnectionstatechange(); };
  setState("disconnected");
  assert.deepEqual(statuses.at(-1), { state: "connecting", message: "Reconnecting…" });
  mock.timers.tick(4000);
  setState("connected");
  assert.deepEqual(statuses.at(-1), { state: "connected", message: "Connected" });
  mock.timers.tick(5000);
  assert.equal(failure, undefined);
  assert.equal(remote.sendControl({ type: "input.tap", x: 0.5, y: 0.5 }), true);

  setState("disconnected");
  mock.timers.tick(5000);
  assert.match(failure?.message ?? "", /Media connection lost/);
});
