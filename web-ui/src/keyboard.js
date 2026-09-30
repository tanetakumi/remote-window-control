const SPECIAL_KEYS = new Set(["Enter", "Tab", "Escape", "Delete", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End", "PageUp", "PageDown"]);

export function attachKeyboardBridge(controls, sendControl, onStatus) {
  const { buttonElement, inputElement, backspaceButton, enterButton } = controls;
  let keyboardActive = false;
  let composing = false;
  let previousValue = "";
  let disposed = false;
  let focusTimer;
  const listeners = [];
  const listen = (element, type, handler, options) => {
    element?.addEventListener(type, handler, options);
    listeners.push(() => element?.removeEventListener(type, handler, options));
  };
  const sendKey = (key) => {
    sendControl({ type: "input.keyDown", key });
    sendControl({ type: "input.keyUp", key });
  };
  const sendText = (text) => {
    const lines = text.split("\n");
    lines.forEach((line, index) => {
      if (index) sendKey("Enter");
      const chars = Array.from(line);
      for (let offset = 0; offset < chars.length; offset += 1024) {
        sendControl({ type: "input.text", text: chars.slice(offset, offset + 1024).join("") });
      }
    });
  };
  const syncUi = () => {
    buttonElement.classList.toggle("active", keyboardActive);
    inputElement.classList.toggle("active", keyboardActive);
  };
  const focus = () => {
    if (disposed || !keyboardActive) return;
    inputElement.focus();
    inputElement.setSelectionRange(inputElement.value.length, inputElement.value.length);
  };
  const commit = () => {
    if (!keyboardActive || composing) return;
    // Keep only a committed tail. Replacements are applied from the first changed character.
    const before = Array.from(previousValue);
    const after = Array.from(inputElement.value);
    let common = 0;
    while (common < before.length && common < after.length && before[common] === after[common]) common++;
    for (let index = common; index < before.length; index++) sendKey("Backspace");
    sendText(after.slice(common).join(""));
    previousValue = inputElement.value;
  };
  listen(buttonElement, "click", () => {
    keyboardActive = !keyboardActive;
    composing = false;
    previousValue = inputElement.value = "";
    syncUi();
    if (keyboardActive) focus(); // iOS focus must stay within the user gesture.
    else inputElement.blur();
    onStatus?.(keyboardActive ? "Keyboard active" : "Keyboard hidden");
  });
  for (const [element, key] of [[backspaceButton, "Backspace"], [enterButton, "Enter"]]) {
    listen(element, "pointerdown", (event) => event.preventDefault());
    listen(element, "click", (event) => {
      event.preventDefault();
      if (composing) return;
      sendKey(key);
      inputElement.value = key === "Enter" ? "" : Array.from(inputElement.value).slice(0, -1).join("");
      previousValue = inputElement.value;
      focus();
    });
  }
  listen(inputElement, "compositionstart", () => { composing = true; });
  listen(inputElement, "compositionend", () => { composing = false; commit(); });
  listen(inputElement, "input", (event) => { if (!event.isComposing) commit(); });
  listen(inputElement, "beforeinput", (event) => {
    if (!composing && keyboardActive && event.inputType === "deleteContentBackward" && !inputElement.value) {
      event.preventDefault();
      sendKey("Backspace");
    }
  });
  listen(inputElement, "keydown", (event) => {
    if (!keyboardActive || composing || event.isComposing || event.keyCode === 229) return;
    if (!SPECIAL_KEYS.has(event.key)) return; // Printable characters, spaces and Backspace use input events.
    event.preventDefault();
    sendKey(event.key);
    // Remote cursor movement invalidates the local replacement context.
    previousValue = inputElement.value = "";
  });
  listen(inputElement, "focus", () => { keyboardActive = true; syncUi(); });
  listen(inputElement, "blur", () => {
    window.clearTimeout(focusTimer);
    if (keyboardActive) focusTimer = window.setTimeout(focus, 0);
  });
  syncUi();
  return {
    isActive: () => keyboardActive,
    cleanup() {
      disposed = true;
      keyboardActive = false;
      window.clearTimeout(focusTimer);
      listeners.forEach((remove) => remove());
      inputElement.blur();
      previousValue = inputElement.value = "";
      syncUi();
    },
  };
}
