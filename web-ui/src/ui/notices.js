// Connection conditions clear independently of a rejected-input notice.
export function createNotices(render) {
  let message = "";
  let transient = "";
  const update = () => render(message || transient);
  return {
    show(text) { message = text; update(); },
    transient(text) { transient = text; update(); },
    dismiss() { if (message) message = ""; else transient = ""; update(); },
    reset() { message = transient = ""; update(); },
  };
}
