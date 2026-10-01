import "./styles.css";

import { fetchWindows, setTargetWindow } from "./core/api.js";
import { createRemoteConnection } from "./core/webrtc.js";
import { attachTouchControlsUI } from "./input/touch-ui.js";
import { attachKeyboardBridge } from "./input/keyboard.js";
import { attachViewportSync, getViewportPayload } from "./input/viewport.js";

const selectScreen = document.querySelector("#select-screen");
const remoteScreen = document.querySelector("#remote-screen");
const selectStatus = document.querySelector("#select-status");
const windowList = document.querySelector("#window-list");
const videoElement = document.querySelector("#remote-video");
const waitingElement = document.querySelector("#video-waiting");
const videoStageElement = document.querySelector("#video-stage");
const touchCursorElement = document.querySelector("#touch-cursor");
const touchModeElement = document.querySelector("#touch-mode");
const statusElement = document.querySelector("#status-pill");
const bitrateElement = document.querySelector("#bitrate-pill");
const keyboardButton = document.querySelector("#keyboard-button");
const hiddenInput = document.querySelector("#hidden-text-input");
const backspaceButton = document.querySelector("#backspace-button");
const enterButton = document.querySelector("#enter-button");
const refreshButton = document.querySelector("#refresh-button");

let connecting = false;
let cleanupRemote = () => {};
let connectionAbort;

function getAppIconSvg() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="20" height="14" x="2" y="3" rx="2"></rect><line x1="8" x2="16" y1="21" y2="21"></line><line x1="12" x2="12" y1="17" y2="21"></line></svg>`;
}

function renderSkeletons(count = 4) {
  windowList.replaceChildren();
  for (let i = 0; i < count; i++) {
    const skeleton = document.createElement("div");
    skeleton.className = "skeleton-card";
    skeleton.setAttribute("aria-hidden", "true");
    windowList.appendChild(skeleton);
  }
}

function renderWindowList(windows, onSelect) {
  windowList.replaceChildren();

  if (windows.length === 0) {
    const emptyState = document.createElement("div");
    emptyState.className = "empty-state";
    emptyState.innerHTML = `
      <p class="empty-title" role="status">No applications available</p>
    `;
    windowList.appendChild(emptyState);
    return;
  }

  for (const target of windows) {
    const card = document.createElement("button");
    card.type = "button";
    card.className = "window-card";
    card.setAttribute(
      "aria-label",
      `Connect to ${target.title || "untitled"} (${target.process_name || "unknown process"})`
    );

    const iconWrapper = document.createElement("div");
    iconWrapper.className = "window-icon-wrapper";
    iconWrapper.innerHTML = getAppIconSvg();
    if (target.icon_png) {
      const icon = document.createElement("img");
      icon.alt = "";
      icon.src = `data:image/png;base64,${target.icon_png}`;
      icon.addEventListener("error", () => {
        iconWrapper.innerHTML = getAppIconSvg();
      }, { once: true });
      iconWrapper.replaceChildren(icon);
    }

    const content = document.createElement("div");
    content.className = "window-card-content";

    const title = document.createElement("span");
    title.className = "window-card-title";
    title.textContent = target.title || "(untitled)";

    const meta = document.createElement("div");
    meta.className = "window-card-meta";

    if (target.process_name) {
      const tag = document.createElement("span");
      tag.className = "process-tag";
      tag.textContent = target.process_name;
      meta.appendChild(tag);
    }

    const sub = document.createElement("span");
    sub.className = "window-card-sub";
    sub.textContent = target.handle ? `ID: ${target.handle}` : "";
    meta.appendChild(sub);

    content.append(title, meta);

    const arrow = document.createElement("div");
    arrow.className = "window-card-arrow";
    arrow.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="9 18 15 12 9 6"></polyline></svg>`;

    card.append(iconWrapper, content, arrow);
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
  const lower = message.toLowerCase();
  if (lower.includes("ready") || lower.includes("connected") || lower.includes("active")) {
    statusElement.dataset.state = "connected";
  } else if (
    lower.includes("fail") ||
    lower.includes("error") ||
    lower.includes("close") ||
    lower.includes("disconnected")
  ) {
    statusElement.dataset.state = "error";
  } else {
    statusElement.dataset.state = "connecting";
  }
}

function setSelectStatus(message, isError = false) {
  selectStatus.textContent = message;
  if (isError) {
    selectStatus.classList.add("has-error");
  } else {
    selectStatus.classList.remove("has-error");
  }
}

async function showWindowSelect() {
  setSelectStatus("");
  refreshButton?.classList.add("is-loading");
  renderSkeletons(4);
  try {
    const windows = await fetchWindows();
    renderWindowList(Array.isArray(windows) ? windows : [], connectToWindow);
  } catch (err) {
    windowList.replaceChildren();
    setSelectStatus(err instanceof Error ? err.message : "Loading failed", true);
  } finally {
    refreshButton?.classList.remove("is-loading");
  }
}

async function connectToWindow(target) {
  if (connecting) return;
  connecting = true;
  setWindowListEnabled(false);
  setSelectStatus(`Connecting to ${target.title || "window"}…`);
  try {
    await setTargetWindow(target.handle);
    selectScreen.hidden = true;
    remoteScreen.hidden = false;
    await startRemoteControl();
  } catch (err) {
    console.error("Connection failed:", err);
    returnToWindows(err instanceof Error ? err.message : "Error", true);
  } finally {
    connecting = false;
  }
}

async function startRemoteControl() {
  cleanupRemote();
  connectionAbort?.abort();
  connectionAbort = new AbortController();
  waitingElement.hidden = false;
  setStatus("Connecting…");
  const remote = await createRemoteConnection({
    getInitialViewport: () => getViewportPayload(videoStageElement),
    signal: connectionAbort.signal,
    videoElement,
    onStatus: setStatus,
    onDisconnect: (error) => returnToWindows(error.message, true),
    onBitrate: (kbps) => {
      bitrateElement.textContent = `${kbps} kbps`;
      bitrateElement.hidden = false;
    },
  });
  waitingElement.hidden = true;
  const releaseTouch = attachTouchControlsUI({
    videoElement,
    stageElement: videoStageElement,
    cursorElement: touchCursorElement,
    modeElement: touchModeElement,
  }, remote.sendControl);
  const keyboard = attachKeyboardBridge(
    {
      buttonElement: keyboardButton,
      inputElement: hiddenInput,
      backspaceButton,
      enterButton,
    },
    remote.sendControl,
    setStatus
  );
  const viewport = attachViewportSync(remote.sendControl, {
    isSuspended: () => keyboard.isActive(),
    targetElement: videoStageElement,
  });
  cleanupRemote = () => {
    releaseTouch();
    keyboard.cleanup();
    viewport.cleanup();
    remote.close();
    bitrateElement.hidden = true;
    cleanupRemote = () => {};
  };
  setStatus("Control ready");
}

function returnToWindows(message = "", isError = false) {
  cleanupRemote();
  connectionAbort?.abort();
  selectScreen.hidden = false;
  remoteScreen.hidden = true;
  setSelectStatus(message, isError);
  setWindowListEnabled(true);
}

videoElement.addEventListener("click", () =>
  videoElement.play()?.catch(() => setStatus("Playback could not start."))
);

document.querySelector("#windows-button").addEventListener("click", () => {
  returnToWindows();
  showWindowSelect();
});

refreshButton?.addEventListener("click", () => {
  if (!connecting) showWindowSelect();
});

window.addEventListener("pagehide", () => returnToWindows());
document.addEventListener("visibilitychange", () => {
  if (document.hidden) returnToWindows();
});

async function main() {
  await showWindowSelect();
}

main();
