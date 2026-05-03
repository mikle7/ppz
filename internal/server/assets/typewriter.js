// ppz landing-page typewriter.
//
// Each <pre data-script="…json…"> on the page gets its `data-script`
// parsed at startup. The script is a list of steps; each step is one
// of:
//
//   {prompt, line, delay}    — render `<prompt> ` then type `line`
//                              char-by-char, then pause `delay`ms
//   {output, delay}          — print `output` instantly (full line),
//                              then pause `delay`ms
//
// Demos cycle: after the last step, sleep ~3 s and start over from
// blank. A click anywhere on a demo article restarts that demo
// immediately (handy for inspection without waiting through a cycle).
(function () {
    "use strict";

    const TYPE_MS = 32;       // per-character typing delay
    const RESTART_MS = 3500;  // pause between cycles

    // Cheap HTML escaper — the demo scripts come from server-side
    // template data we control, but we render via innerHTML now (to
    // colour command rows differently from output rows) so
    // belt-and-braces.
    function esc(s) {
        return s.replace(/[&<>"']/g, c => (
            {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]
        ));
    }

    function buildLine(step, typed) {
        if (step.line !== undefined) {
            const prompt = step.prompt ? step.prompt + " " : "";
            const cursor = typed.length < step.line.length ? "▌" : "";
            // The whole prompt + typed-command row is the user's
            // input, styled in accent so it stands out from the
            // command's output.
            return '<span class="cmd">' + esc(prompt + typed) + '</span>' + cursor;
        }
        return esc(step.output);
    }

    function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

    async function runPane(pane, signal) {
        const script = JSON.parse(pane.dataset.script);
        pane.innerHTML = "";
        const rows = [];
        for (let i = 0; i < script.length; i++) {
            if (signal.aborted) return;
            const step = script[i];
            if (step.line !== undefined) {
                rows.push(buildLine(step, ""));
                for (let n = 1; n <= step.line.length; n++) {
                    rows[rows.length - 1] = buildLine(step, step.line.slice(0, n));
                    pane.innerHTML = rows.join("\n");
                    await sleep(TYPE_MS);
                    if (signal.aborted) return;
                }
                rows[rows.length - 1] = buildLine(step, step.line); // final form, no cursor
                pane.innerHTML = rows.join("\n");
            } else if (step.output !== undefined) {
                rows.push(esc(step.output));
                pane.innerHTML = rows.join("\n");
            }
            await sleep(step.delay || 400);
            if (signal.aborted) return;
        }
    }

    // Fire the "packet flying through ppz" animation between two
    // panes. Adds the `.fire` class to the parent .demo-pair so the
    // packet keyframe runs once, then strips it after the animation
    // completes (1100ms; matches @keyframes packet-fly).
    const PACKET_MS = 1100;
    async function firePacket(pair, signal) {
        if (!pair) return;
        pair.classList.add("fire");
        await sleep(PACKET_MS);
        pair.classList.remove("fire");
        if (signal.aborted) return;
    }

    // Run a demo article. Senders run concurrently (so a "monitor
    // multiple agents" demo shows both agents typing in parallel,
    // not strictly one-after-the-other). When all senders are done
    // we fire one packet animation, then run receivers sequentially.
    // Demos with no senders or no receivers degrade gracefully —
    // they just play in document order.
    async function runDemo(article, signal) {
        const panes = Array.from(article.querySelectorAll(".pane"));
        const senders   = panes.filter(p => p.dataset.pane === "sender");
        const receivers = panes.filter(p => p.dataset.pane === "receiver");

        if (senders.length === 0 && receivers.length === 0) {
            // Untagged panes — just play in order.
            for (const p of panes) {
                if (signal.aborted) return;
                await runPane(p, signal);
            }
            return;
        }

        if (senders.length > 0) {
            await Promise.all(senders.map(s => runPane(s, signal)));
            if (signal.aborted) return;
        }
        if (senders.length > 0 && receivers.length > 0) {
            await firePacket(article, signal);
            if (signal.aborted) return;
        }
        for (const r of receivers) {
            if (signal.aborted) return;
            await runPane(r, signal);
        }
    }

    function loop(article) {
        let controller = new AbortController();
        const cycle = async () => {
            while (true) {
                controller = new AbortController();
                await runDemo(article, controller.signal).catch(() => {});
                if (controller.signal.aborted) continue;
                await sleep(RESTART_MS);
                if (controller.signal.aborted) continue;
            }
        };
        article.addEventListener("click", () => controller.abort());
        cycle();
    }

    // Mark whichever demo is currently in the viewport's centre band
    // as `.in-focus` so the others can fade out (CSS handles the
    // visual). The rootMargin shrinks the viewport to a centre strip:
    // a demo "earns focus" when it crosses into that strip, and loses
    // it when it scrolls past. With threshold 0 + the negative top/
    // bottom margins, transitions feel like the demo "lights up" as
    // it nears the screen centre.
    function wireFocusObserver(articles) {
        if (!("IntersectionObserver" in window) || articles.length === 0) {
            // No IO support — leave them all visible (graceful fallback).
            articles.forEach(a => a.classList.add("in-focus"));
            return;
        }
        const io = new IntersectionObserver((entries) => {
            for (const e of entries) {
                e.target.classList.toggle("in-focus", e.isIntersecting);
            }
        }, { rootMargin: "-30% 0px -30% 0px", threshold: 0 });
        articles.forEach(a => io.observe(a));
    }

    function start() {
        // Match both legacy single-pane (.demo) and paired
        // (.demo-pair) articles. Paired demos animate the packet
        // between sender and receiver; single-pane ones just run.
        const articles = Array.from(document.querySelectorAll(".demo, .demo-pair"));
        articles.forEach(loop);
        wireFocusObserver(articles);
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", start);
    } else {
        start();
    }
})();
