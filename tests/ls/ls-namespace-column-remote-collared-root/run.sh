#!/usr/bin/env bash
# `ppz ls` NAMESPACE column reflects the PIPE's manifold even when the
# pipe was created by a DIFFERENT daemon in the same org. Daemon B
# (alpha-secondary key, same alpha org) creates the source + pipe at
# root with no namespace set; daemon A lists and sees the row with
# NAMESPACE="-".
#
# Locks in: the namespace cell is a property of the pipe, not of the
# listing session.
. /tests/lib/common.sh

ppz_a daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha)"  >/dev/null
ppz_b daemon login "$PPZ_SERVER_URL" -apikey "$(key_alpha2)" >/dev/null

ppz_b unset namespace >/dev/null 2>&1
ppz_b unset handle    >/dev/null 2>&1
ppz_b source create alice >/dev/null
ppz_b pipe create alice.notes >/dev/null

# Daemon A's view is built from the server's GET /api/v1/sources +
# JetStream enrichment; allow up to a beat for JetStream stream
# provisioning to land on the new pipe.
wait_for 20 "ppz_a ls --json | grep -q '\"notes\"'" >/dev/null

ppz_a ls | awk '$2 == "alice.notes" {print "namespace=" $1 " pipe=" $2}'
