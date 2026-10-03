// The host stores the settings; this screen loads them on open and writes
// them back when Save is pressed.
export function attachSettingsScreen({
  fpsInput, fpsValue, crfInput, crfValue, scaleInput, scaleValue, scrollInput, scrollValue, saveButton, statusElement,
  noticeElement, dismissButton,
}, { load, save }) {
  let busy = false;
  let ready = false;
  let active = false;
  let noticeTimer;

  const render = () => {
    saveButton.disabled = busy || !ready;
  };
  const setStatus = (message, isError = false) => {
    clearTimeout(noticeTimer);
    if (!active) return;
    statusElement.textContent = message;
    statusElement.classList.toggle("has-error", isError);
    noticeElement.hidden = !message;
    if (message && !isError) {
      noticeTimer = setTimeout(() => setStatus(""), 5000);
    }
  };
  dismissButton.addEventListener("click", () => setStatus(""));
  const showValues = () => {
    fpsValue.textContent = fpsInput.value;
    crfValue.textContent = crfInput.value;
    scaleValue.textContent = scaleInput.value;
    scrollValue.textContent = `${scrollInput.value}×`;
  };
  const apply = (settings) => {
    fpsInput.value = String(settings.fps);
    crfInput.value = String(settings.crf);
    scaleInput.value = String(settings.maxScale);
    scrollInput.value = String(settings.scrollSensitivity);
    showValues();
  };

  for (const slider of [fpsInput, crfInput, scaleInput, scrollInput]) slider.addEventListener("input", showValues);
  saveButton.addEventListener("click", async () => {
    if (busy || !ready) return;
    busy = true;
    render();
    try {
      apply(await save({
        fps: Number(fpsInput.value),
        crf: Number(crfInput.value),
        maxScale: Number(scaleInput.value),
        scrollSensitivity: Number(scrollInput.value),
      }));
      setStatus("Saved. Reconnect to apply; window scale also updates on viewport changes.");
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
      active = true;
      setStatus("");
      ready = false;
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
    close() {
      setStatus("");
      active = false;
    },
  };
}
