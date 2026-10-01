import "./styles.css";

import { fetchWindows, setTargetWindow } from "./core/api.js";
import { createRemoteConnection } from "./core/webrtc.js";
import { attachGestureControls } from "./input/gestures.js";
import { attachKeyboardBridge } from "./input/keyboard.js";
import { attachViewportSync } from "./input/viewport.js";
import { createListenerTracker } from "./lib/events.js";

const selectScreen = document.querySelector("#select-screen");
const remoteScreen = document.querySelector("#remote-screen");
const selectStatus = document.querySelector("#select-status");
const windowList = document.querySelector("#window-list");
const videoElement = document.querySelector("#remote-video");
const waitingElement = document.querySelector("#video-waiting");
const videoStageElement = document.querySelector("#video-stage");
const statusElement = document.querySelector("#status-pill");
const bitrateElement = document.querySelector("#bitrate-pill");
const fullscreenButton = document.querySelector("#fullscreen-button");
const keyboardButton = document.querySelector("#keyboard-button");
const hiddenInput = document.querySelector("#hidden-text-input");
const backspaceButton = document.querySelector("#backspace-button");
const enterButton = document.querySelector("#enter-button");

let connecting = false;
let cleanupRemote = () => {};
let connectionAbort;

function renderWindowList(windows, onSelect) {
  windowList.replaceChildren();

  for (const target of windows) {
    const card = document.createElement("button");
    card.type = "button";
    card.className = "window-card";

    const title = document.createElement("span");
    title.className = "window-card-title";
    title.textContent = target.title || "(untitled)";

    const sub = document.createElement("span");
    sub.className = "window-card-sub";
    sub.textContent = target.process_name ? ` · ${target.process_name}` : "";

    card.append(title, sub);
    card.addEventListener("click", () => onSelect(target));
    windowList.appendChild(card);
  }
}

function setWindowListEnabled(enabled) {
  for (const button of windowList.querySelectorAll("button")) {
    button.disabled = !enabled;
  }
}

function setStatus(message) {
  statusElement.textContent = message;
}

function attachTopBarControls(onFullscreenChange) {
  const { listen, cleanup } = createListenerTracker();
  listen(fullscreenButton, "click", async () => {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        if (document.documentElement.requestFullscreen) await document.documentElement.requestFullscreen();
        else throw new Error("Fullscreen unavailable");
      }
      onFullscreenChange?.();

    } catch {
      setStatus("Fullscreen is unavailable in this browser.");
    }
  });
  return cleanup;
}

async function showWindowSelect() {
  selectStatus.textContent = "Loading windows…";
  try {
    const windows = await fetchWindows();
    renderWindowList(windows, connectToWindow);
    selectStatus.textContent = windows.length === 0 ? "No windows available" : "";
  } catch (err) {
    selectStatus.textContent = err instanceof Error ? err.message : "Loading failed";
  }
}

async function connectToWindow(target) {
  if (connecting) return;
  connecting = true;
  setWindowListEnabled(false);
  selectStatus.textContent = "Connecting…";
  try {
    await setTargetWindow(target.handle);
    selectScreen.hidden = true;
    remoteScreen.hidden = false;
    await startRemoteControl();
  } catch (err) {
    console.error("Connection failed:", err);
    returnToWindows(err instanceof Error ? err.message : "Error");
  } finally {
    connecting = false;
  }
}

async function startRemoteControl() {
  cleanupRemote();
  connectionAbort?.abort();
  connectionAbort = new AbortController();
  waitingElement.hidden = false;
  const remote = await createRemoteConnection({
    signal: connectionAbort.signal,
    videoElement,
    onStatus: setStatus,
    onDisconnect: (error) => returnToWindows(error.message),
    onBitrate: (kbps) => {
      bitrateElement.textContent = `${kbps} kbps`;
      bitrateElement.hidden = false;
    },
  });
  waitingElement.hidden = true;
  const releaseGestures = attachGestureControls(videoElement, remote.sendControl, setStatus);
  const keyboard = attachKeyboardBridge({
    buttonElement: keyboardButton, inputElement: hiddenInput, backspaceButton, enterButton,
  }, remote.sendControl, setStatus);
  const viewport = attachViewportSync(remote.sendControl, {
    isSuspended: () => keyboard.isActive(), targetElement: videoStageElement,
  });
  const releaseTopBar = attachTopBarControls(viewport.triggerFullscreenSync);
  cleanupRemote = () => {
    releaseGestures();
    keyboard.cleanup();
    viewport.cleanup();
    releaseTopBar();
    remote.close();
    bitrateElement.hidden = true;
    cleanupRemote = () => {};
  };
  setStatus("Control ready");
}

function returnToWindows(message = "Select a window to connect.") {
  cleanupRemote();
  connectionAbort?.abort();
  selectScreen.hidden = false;
  remoteScreen.hidden = true;
  selectStatus.textContent = message;
  setWindowListEnabled(true);
}

videoElement.addEventListener("click", () => videoElement.play()?.catch(() => setStatus("Playback could not start.")));
document.querySelector("#windows-button").addEventListener("click", () => {
  returnToWindows();
  showWindowSelect();
});
document.querySelector("#refresh-button").addEventListener("click", () => { if (!connecting) showWindowSelect(); });
window.addEventListener("pagehide", () => returnToWindows());
document.addEventListener("visibilitychange", () => {
  if (document.hidden) returnToWindows("Connection paused. Select a window to reconnect.");
});

async function main() {
  await showWindowSelect();
}

main();
