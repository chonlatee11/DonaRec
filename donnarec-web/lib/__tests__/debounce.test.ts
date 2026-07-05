import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { debounce } from "@/lib/debounce";

/**
 * debounce — hermetic unit tests (04-08, TemplateEditor's 400ms live-preview
 * debounce, D-61 "ห้าม re-render หนักทุก keystroke").
 */
describe("debounce", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("only invokes the wrapped function once after the wait elapses following the LAST call", () => {
    const fn = vi.fn();
    const debounced = debounce(fn, 400);

    debounced("a");
    vi.advanceTimersByTime(200);
    debounced("b");
    vi.advanceTimersByTime(200);
    debounced("c");
    vi.advanceTimersByTime(399);
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenCalledWith("c");
  });

  it("invokes the function again on a subsequent call after the previous debounce settled", () => {
    const fn = vi.fn();
    const debounced = debounce(fn, 400);

    debounced("first");
    vi.advanceTimersByTime(400);
    expect(fn).toHaveBeenCalledTimes(1);

    debounced("second");
    vi.advanceTimersByTime(400);
    expect(fn).toHaveBeenCalledTimes(2);
    expect(fn).toHaveBeenLastCalledWith("second");
  });

  it("cancel() prevents a pending invocation", () => {
    const fn = vi.fn();
    const debounced = debounce(fn, 400);

    debounced("x");
    debounced.cancel();
    vi.advanceTimersByTime(1000);

    expect(fn).not.toHaveBeenCalled();
  });
});
