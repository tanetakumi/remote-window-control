import { createListenerTracker } from "../lib/events.js";

// 1024 Unicode code points fit the host's 4096-byte text limit, including emoji.
function* textCommands(text, shiftNewline) {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  for (const [index, line] of lines.entries()) {
    if (index) {
      if (shiftNewline) yield { type: "input.keyDown", key: "Shift" };
      yield { type: "input.keyDown", key: "Enter" };
      yield { type: "input.keyUp", key: "Enter" };
      if (shiftNewline) yield { type: "input.keyUp", key: "Shift" };
    }
    const chars = Array.from(line);
    for (let offset = 0; offset < chars.length; offset += 1024) {
      yield { type: "input.text", text: chars.slice(offset, offset + 1024).join("") };
    }
  }
}

export function attachTextInput({
  buttonElement, dialogElement, inputElement, closeButton, sendButton,
  restoreButton, enterButton, shiftEnterButton, errorElement, draft, onOpenChange,
}, sendControl) {
  const { listen, cleanup: removeListeners } = createListenerTracker();
  let active = false;
  let composing = false;
  let sending = false;
  let disposed = false;
  let backdropPress = false;
  inputElement.value = draft.text;

  const syncUi = () => {
    buttonElement.classList.toggle("active", active);
    buttonElement.setAttribute("aria-expanded", String(active));
    sendButton.disabled = disposed || !active || composing || sending || inputElement.value.length === 0;
    if (errorElement.textContent !== draft.error) errorElement.textContent = draft.error;
    errorElement.hidden = !draft.error;
    enterButton.setAttribute("aria-pressed", String(!draft.shiftNewline));
    shiftEnterButton.setAttribute("aria-pressed", String(!!draft.shiftNewline));
    restoreButton.hidden = !draft.error || !draft.lastSent || inputElement.value.length > 0;
  };
  const positionDialog = () => {
    if (!active) return;
    const viewport = window.visualViewport;
    dialogElement.style.setProperty("--visible-top", `${viewport?.offsetTop ?? 0}px`);
    dialogElement.style.setProperty("--visible-left", `${viewport?.offsetLeft ?? 0}px`);
    dialogElement.style.setProperty("--visible-width", `${viewport?.width ?? window.innerWidth}px`);
    dialogElement.style.setProperty("--visible-height", `${viewport?.height ?? window.innerHeight}px`);
  };
  const finishClosing = (restoreFocus = true) => {
    if (!active) return;
    draft.text = inputElement.value;
    active = composing = false;
    inputElement.blur();
    syncUi();
    onOpenChange?.(false);
    // iOS may leave the page scrolled after the keyboard closes.
    if (!disposed) window.scrollTo?.(0, 0);
    if (restoreFocus && !disposed) buttonElement.focus({ preventScroll: true });
  };
  const close = () => {
    dialogElement.close();
    finishClosing();
  };
  const open = () => {
    if (disposed || active) return;
    active = true;
    positionDialog();
    dialogElement.showModal();
    syncUi();
    onOpenChange?.(true);
    // Keep focus inside the click handler so iOS can open its software keyboard.
    inputElement.focus({ preventScroll: true });
  };
  const reportError = (message) => {
    draft.error = message;
    syncUi();
  };

  listen(buttonElement, "click", open);
  const keepFocus = (event) => {
    // Keep the textarea focused until click. Blurring on a button press
    // dismisses the mobile keyboard, and refocusing afterwards makes iOS
    // scroll the page out from under the top bar.
    if (active && event.pointerType === "touch" && event.isPrimary && event.button === 0) {
      event.preventDefault();
    }
  };
  for (const button of [closeButton, enterButton, shiftEnterButton, sendButton, restoreButton]) {
    listen(button, "pointerdown", keepFocus);
  }
  listen(closeButton, "click", close);
  listen(dialogElement, "close", () => {
    // A queued close event must not close an editor that has already reopened.
    if (!dialogElement.open) finishClosing();
  });
  listen(dialogElement, "cancel", (event) => {
    if (composing) event.preventDefault();
  });
  const outsideDialog = (event) => {
    const rect = dialogElement.getBoundingClientRect();
    return event.clientX < rect.left || event.clientX > rect.right
      || event.clientY < rect.top || event.clientY > rect.bottom;
  };
  listen(dialogElement, "pointerdown", (event) => {
    backdropPress = event.target === dialogElement && outsideDialog(event);
  });
  listen(dialogElement, "click", (event) => {
    if (backdropPress && event.target === dialogElement && outsideDialog(event)) close();
    backdropPress = false;
  });
  listen(inputElement, "compositionstart", () => { composing = true; syncUi(); });
  listen(inputElement, "compositionend", () => {
    composing = false;
    draft.text = inputElement.value;
    syncUi();
  });
  listen(inputElement, "input", () => {
    draft.text = inputElement.value;
    syncUi();
  });
  for (const [button, shift] of [[enterButton, false], [shiftEnterButton, true]]) {
    listen(button, "click", () => {
      draft.shiftNewline = shift;
      syncUi();
    });
  }
  listen(restoreButton, "click", () => {
    inputElement.value = draft.text = draft.lastSent;
    syncUi();
    inputElement.focus({ preventScroll: true });
  });
  listen(sendButton, "click", () => {
    if (disposed || !active || composing || sending || !inputElement.value) return;
    sending = true;
    draft.text = draft.lastSent = inputElement.value;
    draft.error = "";
    syncUi();
    try {
      for (const command of textCommands(draft.text, draft.shiftNewline)) {
        if (disposed || !sendControl(command)) {
          reportError("Text may have been partially sent. Check the remote window before retrying.");
          return;
        }
      }
      if (disposed) return;
      inputElement.value = draft.text = "";
      close();
    } finally {
      sending = false;
      syncUi();
    }
  });
  listen(window, "resize", positionDialog);
  if (window.visualViewport) {
    listen(window.visualViewport, "resize", positionDialog);
    listen(window.visualViewport, "scroll", positionDialog);
  }
  buttonElement.disabled = false;
  syncUi();

  return {
    isActive: () => active,
    reportError,
    cleanup() {
      disposed = true;
      buttonElement.disabled = true;
      removeListeners();
      dialogElement.close();
      finishClosing(false);
    },
  };
}
