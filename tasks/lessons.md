# Lessons

- When adding modal or pending-confirmation state to a Bubble Tea screen
  (e.g. a y/n intercept in a key handler), always check how mouse clicks
  interact with it: handleMouse synthesizes key presses via clickAction(), so
  a click can be misread as an arbitrary "not y" keypress or accidentally
  fire the row action. Decide explicitly what a click means while the modal
  state is pending and add a handleMouse-level test for it. (Caught by the
  v1.4.0 code review: connection-removal confirm vs mouse click.)
- When a test fake generates sequential ids (graphtest.Fake returns new-N per
  POST), do not infer id-to-call mappings from call order under a concurrent
  worker pool; make ids deterministic via PostFunc keyed on request content.
- A cache that releases its lock during the fetch needs reserve-before-fetch
  (insert a placeholder under the lock) if tests or costs require exactly one
  fetch per key under concurrency.
