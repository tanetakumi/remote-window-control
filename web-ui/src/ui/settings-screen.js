const NEWLINE_ENTER = "enter";
const NEWLINE_SHIFT_ENTER = "shift-enter";

// The host stores the settings; this screen loads them on open and writes
// them back when Save is pressed.
export function attachSettingsScreen({
  enterButton, shiftEnterButton, fpsInput, fpsValue, crfInput, crfValue, saveButton, statusElement,
}, { load, save }) {
  let newline = NEWLINE_ENTER;
  let busy = false;
  let ready = false;

  const render = () => {
    enterButton.setAttribute("aria-pressed", String(newline === NEWLINE_ENTER));
    shiftEnterButton.setAttribute("aria-pressed", String(newline === NEWLINE_SHIFT_ENTER));
    saveButton.disabled = busy || !ready;
  };
  const setStatus = (message, isError = false) => {
    statusElement.textContent = message;
    statusElement.classList.toggle("has-error", isError);
  };
  const showValues = () => {
    fpsValue.textContent = fpsInput.value;
    crfValue.textContent = crfInput.value;
  };
  const apply = (settings) => {
    newline = settings.newline;
    fpsInput.value = String(settings.fps);
    crfInput.value = String(settings.crf);
    showValues();
  };

  for (const [button, value] of [[enterButton, NEWLINE_ENTER], [shiftEnterButton, NEWLINE_SHIFT_ENTER]]) {
    button.addEventListener("click", () => {
      newline = value;
      render();
    });
  }
  for (const slider of [fpsInput, crfInput]) slider.addEventListener("input", showValues);
  saveButton.addEventListener("click", async () => {
    if (busy || !ready) return;
    busy = true;
    setStatus("Saving…");
    render();
    try {
      apply(await save({ fps: Number(fpsInput.value), crf: Number(crfInput.value), newline }));
      setStatus("Saved. New FPS and CRF values apply from the next connection.");
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "Saving failed", true);
    } finally {
      busy = false;
      render();
    }
  });

  render();
  return {
    // Reload the stored values, discarding unsaved edits.
    async open() {
      ready = false;
      setStatus("Loading…");
      render();
      try {
        apply(await load());
        ready = true;
        setStatus("");
      } catch (err) {
        setStatus(err instanceof Error ? err.message : "Loading failed", true);
      }
      render();
    },
  };
}
