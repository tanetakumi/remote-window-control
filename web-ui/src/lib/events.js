export function createListenerTracker() {
  const removers = [];

  return {
    listen(target, type, handler, options) {
      target.addEventListener(type, handler, options);
      removers.push(() => target.removeEventListener(type, handler, options));
    },
    cleanup() {
      for (const remove of removers.splice(0)) remove();
    },
  };
}
