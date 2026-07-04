/**
 * debounce — generic trailing-edge debounce utility.
 *
 * Backs TemplateEditor's 400ms-debounced live preview (D-61: "UX ต้องสมูทที่สุด —
 * ห้าม re-render หนักทุก keystroke; ใช้ debounce/throttle"). Implemented as a plain
 * function (not a React hook) so it is testable in a hermetic node environment with
 * fake timers, independent of React/jsdom.
 */
export function debounce<Args extends unknown[]>(
  fn: (...args: Args) => void,
  waitMs: number
): ((...args: Args) => void) & { cancel: () => void } {
  let timer: ReturnType<typeof setTimeout> | null = null;

  const debounced = (...args: Args): void => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      fn(...args);
    }, waitMs);
  };

  debounced.cancel = (): void => {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  };

  return debounced;
}
