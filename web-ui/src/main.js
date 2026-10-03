import "./styles.css";
import { renderInputMode } from "./ui/input-mode.js";
import { createNotices } from "./ui/notices.js";

import {
  fetchWindows, setTargetWindow, fetchKeepalive, setKeepalive, fetchSettings, saveSettings,
} from "./core/api.js";
import { createRemoteConnection } from "./core/webrtc.js";
import { attachTouchControlsUI } from "./input/touch-ui.js";
import { attachTextInput } from "./input/keyboard.js";
import { attachSpecialKeysPalette } from "./input/special-keys-palette.js";
import { attachSettingsScreen } from "./ui/settings-screen.js";
import { attachViewportSync, getViewportPayload } from "./input/viewport.js";

const selectScreen = document.querySelector("#select-screen");
const settingsScreen = document.querySelector("#settings-screen");
const remoteScreen = document.querySelector("#remote-screen");
const selectStatus = document.querySelector("#select-status");
const windowList = document.querySelector("#window-list");
const videoElement = document.querySelector("#remote-video");
const waitingElement = document.querySelector("#video-waiting");
const videoStageElement = document.querySelector("#video-stage");
const touchCursorElement = document.querySelector("#touch-cursor");
const touchModeElement = document.querySelector("#touch-mode");
const inputModeButton = document.querySelector("#input-mode-button");
const statusMessageElement = document.querySelector("#status-message");
const bitrateElement = document.querySelector("#bitrate-pill");
const bitrateValue = bitrateElement.querySelector(".bitrate-value");
const keyboardButton = document.querySelector("#keyboard-button");
const specialKeysButton = document.querySelector("#special-keys-button");
const textDialog = document.querySelector("#text-dialog");
const textInput = document.querySelector("#remote-text-input");
const noticeElement = document.querySelector("#remote-notice");
const noticeMessage = document.querySelector("#remote-notice-message");
const refreshButton = document.querySelector("#refresh-button");
const settingsButton = document.querySelector("#settings-button");
const keepaliveButton = document.querySelector("#keepalive-button");
const keepaliveDot = document.querySelector("#keepalive-dot");

const notices = createNotices((message) => {
  noticeMessage.textContent = message;
  noticeElement.hidden = !message;
});
let connecting = false;
let keepaliveBusy = false;
let cleanupRemote = () => {};
let connectionAbort;
// Keep only the current target's draft in memory, including across reconnects.
let textDraft = { target: null, text: "", lastSent: "", error: "" };

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

function setStatus(state, message) {
  statusMessageElement.textContent = message;
  notices.transient(state === "connecting" ? message : "");
}

function showNotice(message) {
  statusMessageElement.textContent = message;
  notices.show(message);
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
  try {
    await setTargetWindow(target.handle);
    if (textDraft.target !== target.handle) {
      textDraft = { target: target.handle, text: "", lastSent: "", error: "" };
    }
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
  notices.reset();
  renderInputMode(inputModeButton);
  keyboardButton.disabled = true;
  specialKeysButton.disabled = true;
  setStatus("connecting", "Connecting…");
  const { scrollSensitivity } = await fetchSettings();
  let keyboard;
  let viewport;
  const remote = await createRemoteConnection({
    getInitialViewport: () => getViewportPayload(videoStageElement),
    signal: connectionAbort.signal,
    videoElement,
    onStatus: setStatus,
    onInputMode: (mode, switching) => {
      renderInputMode(inputModeButton, mode, switching, true);
      if (!switching) viewport?.refresh();
    },
    onNotice: showNotice,
    onInputError: (message) => {
      showNotice(message);
      keyboard?.reportError(message);
    },
    onDisconnect: (error) => returnToWindows(error.message, true),
    onBitrate: (kbps) => {
      bitrateValue.textContent = String(kbps);
      bitrateElement.hidden = false;
    },
  });
  waitingElement.hidden = true;
  const switchMode = () => remote.setInputMode(remote.inputMode === "pc" ? "window" : "pc");
  inputModeButton.addEventListener("click", switchMode);
  const releaseTouch = attachTouchControlsUI({
    videoElement,
    stageElement: videoStageElement,
    cursorElement: touchCursorElement,
    modeElement: touchModeElement,
  }, remote.sendControl, { scrollSensitivity });
  const specialKeys = attachSpecialKeysPalette({
    buttonElement: specialKeysButton,
    paletteElement: document.querySelector("#special-keys-palette"),
    stageElement: videoStageElement,
    moveHandle: document.querySelector("#move-special-keys-palette"),
    keyButtons: [
      { element: document.querySelector("#send-backspace"), keys: ["Backspace"] },
      { element: document.querySelector("#send-shift-enter"), keys: ["Shift", "Enter"] },
      { element: document.querySelector("#send-arrow-up"), keys: ["ArrowUp"] },
      { element: document.querySelector("#send-arrow-left"), keys: ["ArrowLeft"] },
      { element: document.querySelector("#send-arrow-down"), keys: ["ArrowDown"] },
      { element: document.querySelector("#send-arrow-right"), keys: ["ArrowRight"] },
    ],
  }, remote.sendControl);
  keyboard = attachTextInput(
    {
      buttonElement: keyboardButton,
      dialogElement: textDialog,
      inputElement: textInput,
      closeButton: document.querySelector("#close-text-input"),
      sendButton: document.querySelector("#send-text-input"),
      sendEnterButton: document.querySelector("#send-enter-text-input"),
      restoreButton: document.querySelector("#restore-text-input"),
      errorElement: document.querySelector("#text-input-error"),
      draft: textDraft,
      onOpenChange: (active) => {
        specialKeys.setTextInputActive(active);
        viewport?.refresh();
      },
    },
    remote.sendControl
  );
  viewport = attachViewportSync(remote.sendControl, {
    isSuspended: () => keyboard.isActive(),
    targetElement: videoStageElement,
  });
  cleanupRemote = () => {
    inputModeButton.removeEventListener("click", switchMode);
    renderInputMode(inputModeButton);
    releaseTouch();
    specialKeys.cleanup();
    keyboard.cleanup();
    viewport.cleanup();
    remote.close();
    bitrateElement.hidden = true;
    cleanupRemote = () => {};
  };
}

function returnToWindows(message = "", isError = false) {
  cleanupRemote();
  connectionAbort?.abort();
  selectScreen.hidden = false;
  settingsScreen.hidden = true;
  remoteScreen.hidden = true;
  setSelectStatus(message, isError);
  setWindowListEnabled(true);
}

videoElement.addEventListener("click", () =>
  videoElement.play()?.catch(() => showNotice("Playback could not start. Tap the video to retry."))
);

document.querySelector("#dismiss-notice").addEventListener("click", () => {
  notices.dismiss();
});

document.querySelector("#windows-button").addEventListener("click", () => {
  returnToWindows();
  showWindowSelect();
});

refreshButton?.addEventListener("click", () => {
  if (!connecting) showWindowSelect();
});

const settings = attachSettingsScreen({
  fpsInput: document.querySelector("#settings-fps"),
  fpsValue: document.querySelector("#settings-fps-value"),
  crfInput: document.querySelector("#settings-crf"),
  crfValue: document.querySelector("#settings-crf-value"),
  scaleInput: document.querySelector("#settings-scale"),
  scaleValue: document.querySelector("#settings-scale-value"),
  scrollInput: document.querySelector("#settings-scroll"),
  scrollValue: document.querySelector("#settings-scroll-value"),
  saveButton: document.querySelector("#settings-save"),
  statusElement: document.querySelector("#settings-status"),
}, { load: fetchSettings, save: saveSettings });

settingsButton.addEventListener("click", () => {
  if (connecting) return;
  selectScreen.hidden = true;
  settingsScreen.hidden = false;
  settings.open();
});

document.querySelector("#settings-back").addEventListener("click", () => {
  settingsScreen.hidden = true;
  selectScreen.hidden = false;
});

function renderKeepalive(status) {
  if (!keepaliveDot) return;
  keepaliveDot.dataset.state = status?.state ?? "off";
  if (keepaliveButton) {
    keepaliveButton.setAttribute("aria-pressed", String(Boolean(status?.enabled)));
    keepaliveButton.title = status?.error || "Keep the session display alive";
  }
}

async function refreshKeepalive() {
  try {
    renderKeepalive(await fetchKeepalive());
  } catch {
    renderKeepalive(null);
  }
}

keepaliveButton?.addEventListener("click", async () => {
  if (keepaliveBusy) return;
  keepaliveBusy = true;
  try {
    const status = await fetchKeepalive();
    renderKeepalive(await setKeepalive(!status.enabled));
  } catch (err) {
    setSelectStatus(err instanceof Error ? err.message : "Keep-alive failed", true);
  } finally {
    keepaliveBusy = false;
  }
});
// Poll only while the window list (where the toggle lives) is shown, and not
// while a toggle is in flight so a stale poll cannot overwrite its result.
setInterval(() => {
  if (!document.hidden && !selectScreen.hidden && !keepaliveBusy) refreshKeepalive();
}, 5000);
refreshKeepalive();

window.addEventListener("pagehide", () => returnToWindows());
document.addEventListener("visibilitychange", () => {
  if (document.hidden) returnToWindows();
});

async function main() {
  await showWindowSelect();
}

main();
