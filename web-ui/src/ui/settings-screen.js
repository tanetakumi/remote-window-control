// The host stores the settings; this screen loads them on open and writes
// them back when Save is pressed.
export function attachSettingsScreen({
  fpsInput, fpsValue, crfInput, crfValue, scaleInput, scaleValue, scrollInput, scrollValue, saveButton, statusElement,
}, { load, save }) {
  let busy = false;
  let ready = false;

  const render = () => {
    saveButton.disabled = busy || !ready;
  };
  const setStatus = (message, isError = false) => {
    statusElement.textContent = message;
    statusElement.classList.toggle("has-error", isError);
  };
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
      setStatus("Saved. FPS, CRF and scroll sensitivity apply from the next connection; max window scale applies from the next viewport change or connection.");
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
