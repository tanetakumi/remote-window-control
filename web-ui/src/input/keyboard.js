import { createListenerTracker } from "../lib/events.js";

export function attachTextInput({
  buttonElement, dialogElement, inputElement, closeButton, sendButton,
  sendEnterButton, restoreButton, errorElement, draft, onOpenChange,
}, sendControl) {
  const { listen, cleanup: removeListeners } = createListenerTracker();
  let active = false;
  let composing = false;
  let sending = false;
  let disposed = false;
  let backdropPress = false;
  inputElement.value = draft.text;

  const syncUi = () => {
    buttonElement.setAttribute("aria-expanded", String(active));
    sendButton.disabled = sendEnterButton.disabled =
      disposed || !active || composing || sending || inputElement.value.length === 0;
    if (errorElement.textContent !== draft.error) errorElement.textContent = draft.error;
    errorElement.hidden = !draft.error;
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
  for (const button of [closeButton, sendButton, sendEnterButton, restoreButton]) {
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
  listen(restoreButton, "click", () => {
    inputElement.value = draft.text = draft.lastSent;
    syncUi();
    inputElement.focus({ preventScroll: true });
  });
  const send = (enter) => {
    if (disposed || !active || composing || sending || !inputElement.value) return;
    sending = true;
    draft.text = draft.lastSent = inputElement.value;
    draft.error = "";
    syncUi();
    try {
      // The host pastes the whole text at once, line breaks included. Enter
      // rides in the same command so the host can press it after the paste.
      const command = { type: "input.text", text: draft.text };
      if (enter) command.enter = true;
      if (!sendControl(command)) {
        reportError("Text could not be sent.");
        return;
      }
      if (disposed) return;
      inputElement.value = draft.text = "";
      close();
    } finally {
      sending = false;
      syncUi();
    }
  };
  listen(sendButton, "click", () => send(false));
  listen(sendEnterButton, "click", () => send(true));
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
