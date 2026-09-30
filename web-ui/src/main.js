import "./styles.css";

import { bootstrapAuth, clearToken, getStoredToken } from "./auth.js";
import { fetchWindows, setTargetWindow } from "./api.js";

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}
import { attachGestureControls } from "./gestures.js";
import { attachInstallPrompt } from "./install-prompt.js";
import { attachKeyboardBridge } from "./keyboard.js";
import { attachViewportSync } from "./viewport.js";
import { createRemoteConnection } from "./webrtc.js";

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

function setStatus(message) {
  statusElement.textContent = message;
}

function attachTopBarControls(onFullscreenChange) {
  const controller = new AbortController();
  fullscreenButton?.addEventListener("click", async () => {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        if (document.documentElement.requestFullscreen) await document.documentElement.requestFullscreen();
        else throw new Error("Fullscreen unavailable");
      }
      onFullscreenChange?.();

    } catch {
      setStatus("Fullscreen is unavailable in this browser. Use the installed app for more screen space.");
    }
  }, { signal: controller.signal });
  return () => controller.abort();
}

function escapeHtml(str) {
  return String(str)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function showWindowSelect(token) {
  try {
    if (selectStatus) selectStatus.textContent = "Loading windows…";
    const windows = await fetchWindows(token);

    if (windowList) windowList.innerHTML = "";
    if (windows.length === 0) {
      if (selectStatus) selectStatus.textContent = "No windows available";
      return;
    }
    if (selectStatus) selectStatus.textContent = "";
    for (const w of windows) {
      const card = document.createElement("button");
      card.type = "button";
      card.className = "window-card";
      const title = w.title || "(untitled)";
      const sub = w.process_name ? ` · ${w.process_name}` : "";
      card.innerHTML = `
        <span class="window-card-title">${escapeHtml(title)}</span>
        <span class="window-card-sub">${escapeHtml(sub)}</span>
      `;
      card.addEventListener("click", async () => {
        if (connecting) return;
        connecting = true;
        windowList.querySelectorAll("button").forEach((button) => { button.disabled = true; });
        if (selectStatus) selectStatus.textContent = "Connecting…";
        try {
          await setTargetWindow(getStoredToken() ?? token, w.handle);
          if (selectScreen) selectScreen.hidden = true;
          if (remoteScreen) remoteScreen.hidden = false;
          await startRemoteControl(getStoredToken());
        } catch (err) {
          console.error("Connection failed:", err);
          if (selectScreen) selectScreen.hidden = false;
          if (remoteScreen) remoteScreen.hidden = true;
          const msg = err instanceof Error ? err.message : "Error";
          if (selectStatus) selectStatus.textContent = msg;
          if (statusElement) statusElement.textContent = msg;
          windowList.querySelectorAll("button").forEach((button) => { button.disabled = false; });
        } finally {
          connecting = false;
        }
      });
      windowList?.appendChild(card);
    }
  } catch (err) {
    if (selectStatus) selectStatus.textContent = err instanceof Error ? err.message : "Loading failed";
  }
}

let connecting = false;
let cleanupRemote = () => {};
let connectionAbort;

async function startRemoteControl(token) {
  cleanupRemote();
  connectionAbort?.abort();
  connectionAbort = new AbortController();
  waitingElement.hidden = false;
  const remote = await createRemoteConnection({
    token,
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

videoElement.addEventListener("click", () => videoElement.play()?.catch(() => setStatus("Playback could not start.")));
function returnToWindows(message = "Select a window to connect.") {
  cleanupRemote();
  connectionAbort?.abort();
  selectScreen.hidden = false;
  remoteScreen.hidden = true;
  selectStatus.textContent = message;
  windowList.querySelectorAll("button").forEach((button) => { button.disabled = false; });
}
document.querySelector("#windows-button").addEventListener("click", () => {
  returnToWindows();
  showWindowSelect(getStoredToken());
});
document.querySelector("#refresh-button").addEventListener("click", () => { if (!connecting) showWindowSelect(getStoredToken()); });
window.addEventListener("pagehide", () => returnToWindows());
document.addEventListener("visibilitychange", () => {
  if (document.hidden) returnToWindows("Connection paused. Select a window to reconnect.");
});

async function main() {
  attachInstallPrompt(document.body, (msg) => {
    if (selectStatus) selectStatus.textContent = msg;
    if (statusElement) statusElement.textContent = msg;
  });

  try {
    const token = await bootstrapAuth();
    if (!token) {
      if (selectStatus) selectStatus.textContent = "Open the host link containing the secret parameter.";
      if (statusElement) statusElement.textContent = "Open the host link containing the secret parameter.";
      return;
    }

    await showWindowSelect(token);
  } catch (error) {
    clearToken();
    if (selectStatus) selectStatus.textContent = error instanceof Error ? error.message : "Error";
  }
}

main();
