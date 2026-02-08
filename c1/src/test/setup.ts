import '@testing-library/jest-dom/vitest';

// ---------------------------------------------------------------------------
// Layout mocks for @tanstack/react-virtual in JSDOM
// ---------------------------------------------------------------------------

const MOCK_HEIGHT = 600;
const MOCK_WIDTH = 320;

// 1. ResizeObserver mock — deferred callback to avoid infinite loops
//    with virtualizer's _measureElement → observe → callback cycle.
class MockResizeObserver {
  private callback: ResizeObserverCallback;
  private targets: Set<Element> = new Set();

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }

  observe(target: Element) {
    if (this.targets.has(target)) return;
    this.targets.add(target);
    // Defer to next microtask to break synchronous recursion
    Promise.resolve().then(() => {
      this.callback(
        [
          {
            target,
            contentRect: { width: MOCK_WIDTH, height: MOCK_HEIGHT } as DOMRectReadOnly,
            borderBoxSize: [{ blockSize: MOCK_HEIGHT, inlineSize: MOCK_WIDTH }] as any,
            contentBoxSize: [{ blockSize: MOCK_HEIGHT, inlineSize: MOCK_WIDTH }] as any,
            devicePixelContentBoxSize: [] as any,
          },
        ],
        this as any,
      );
    });
  }

  unobserve(target: Element) {
    this.targets.delete(target);
  }

  disconnect() {
    this.targets.clear();
  }
}

(globalThis as any).ResizeObserver = MockResizeObserver;

// 2. getBoundingClientRect — returns plausible dimensions
Element.prototype.getBoundingClientRect = function () {
  return {
    width: MOCK_WIDTH,
    height: MOCK_HEIGHT,
    top: 0,
    left: 0,
    bottom: MOCK_HEIGHT,
    right: MOCK_WIDTH,
    x: 0,
    y: 0,
    toJSON: () => {},
  } as DOMRect;
};

// 3. clientHeight / scrollHeight / offsetHeight
for (const prop of ['clientHeight', 'scrollHeight', 'offsetHeight'] as const) {
  Object.defineProperty(HTMLElement.prototype, prop, {
    configurable: true,
    get() { return MOCK_HEIGHT; },
  });
}

for (const prop of ['clientWidth', 'scrollWidth', 'offsetWidth'] as const) {
  Object.defineProperty(HTMLElement.prototype, prop, {
    configurable: true,
    get() { return MOCK_WIDTH; },
  });
}
