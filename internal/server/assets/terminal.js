// Read-only live terminal viewer.
//
// Mounts xterm.js on #terminal, opens a binary WebSocket against the path
// in data-ws-path, and writes every received frame straight into the
// emulator. No stdin yet — keypresses on the canvas are dropped on the
// floor (xterm.js writes locally regardless of attached input; we don't
// hook onData, so user typing doesn't echo).

(function () {
  const root = document.getElementById("terminal");
  if (!root) return;

  const wsPath = root.getAttribute("data-ws-path");
  if (!wsPath) return;

  // Pick xterm.js colours so the empty viewport (rows beyond rendered
  // content — a 105×57 pty with 6 lines of content has ~50 blank rows)
  // blends into the page rather than reading as a giant black slab.
  // Match style.css's --bg / --text via prefers-color-scheme so the
  // terminal follows the site's light/dark mode.
  const dark = window.matchMedia &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
  const theme = dark
    ? { background: "#1a1a18", foreground: "#ececea", cursor: "#ececea" }
    : { background: "#fafaf9", foreground: "#1c1c1a", cursor: "#1c1c1a" };

  const term = new Terminal({
    convertEol: false,            // keep raw \r\n from the pty
    cursorBlink: false,
    disableStdin: true,           // read-only
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    fontSize: 13,
    scrollback: 5000,
    theme,
  });
  term.open(root);

  const status = document.getElementById("terminal-status");
  function setStatus(text, kind) {
    if (!status) return;
    status.textContent = text;
    status.className = "terminal-status " + (kind || "");
  }

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const url = proto + "//" + location.host + wsPath;
  const ws = new WebSocket(url);
  ws.binaryType = "arraybuffer";

  ws.onopen = () => setStatus("connected — streaming live", "ok");
  ws.onerror = () => setStatus("connection error", "err");
  ws.onclose = (ev) => setStatus(
    "disconnected (code " + ev.code + ")", "err");
  ws.onmessage = (ev) => {
    if (typeof ev.data === "string") {
      // Text frames carry control signals from <handle>.stdctrl as JSON.
      // Dispatch by type; ignore unknown types so the wire can grow
      // without breaking the viewer.
      try {
        const ctrl = JSON.parse(ev.data);
        if (ctrl && ctrl.type === "resize" &&
            Number.isFinite(ctrl.cols) && Number.isFinite(ctrl.rows) &&
            ctrl.cols > 0 && ctrl.rows > 0) {
          term.resize(ctrl.cols, ctrl.rows);
        }
      } catch (_) { /* malformed json — drop */ }
    } else {
      term.write(new Uint8Array(ev.data));
    }
  };
})();
