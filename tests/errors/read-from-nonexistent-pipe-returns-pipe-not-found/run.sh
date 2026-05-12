#!/usr/bin/env bash
# RED: reading from a custom pipe that doesn't exist (no `ppz pipe create`
# was run on it) must surface as E_PIPE_NOT_FOUND — NOT E_INVALID_PIPE.
#
# Sibling bug to the one PR #39 fixed in `handleBroadcast` (`ppz send`).
# Today, `handleRead` (`ppz read` / `ppz reread`) has the same shape:
#
#     internal/daemon/read.go:83-91
#     stream, err := js.Stream(ctx, streamName)
#     if err != nil {
#         // ...comment says "No stream means the pipe has never been
#         // provisioned... Either way, return E_INVALID_PIPE."
#         writeReadErr(conn, cliproto.New(cliproto.EInvalidPipe))
#         return
#     }
#
# Same conflation: stream-not-found and NATS-unreachable both collapse
# to E_INVALID_PIPE. The fix is the same — classify via errors.Is on
# jetstream.ErrStreamNotFound / context.DeadlineExceeded / nats.* errors
# and route to E_PIPE_NOT_FOUND / E_NATS_UNREACHABLE respectively, with
# E_INVALID_PIPE staying as the catch-all for truly unexpected errors.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)" >/dev/null
ppz_a terminal create chat >/dev/null

# `chat` exists; `chat.nope` does not. `nope` is a valid pipe name
# (passes natsubj.ValidatePipe regex), so we reach the JetStream
# stream-existence check where the bug lives.
err_code=$(ppz_a read chat.nope 2>&1 | grep -oE "^error: E_[A-Z_]+" | head -1 | sed 's/^error: //')
echo "error_code=$err_code"
