#!/bin/sh
# fake-claude-truncated.sh — hermetic `claude` stand-in for the HRD-127
# CHAOS (c) TRUNCATED-REPLY scenario: the subprocess exits 0 (claude
# "succeeded") but emits a CORRUPT reply — the <<<HERALD-REPLY>>> marker is
# present, an opening '{' follows, but the JSON object is cut off
# mid-stream (no closing brace, missing fields). This is the classic
# "stream died after the marker" corruption.
#
# Asserts the §107 anti-bluff contract: production parseReply() MUST return
# an explicit "decode reply JSON" error — it must NEVER silently
# partial-accept the truncated object or synthesise a default
# DispatchResponse. Exit 0 alone is NOT a PASS; a well-formed parsed reply
# is required.
printf '<<<HERALD-REPLY>>> {"outcome":"answered","summary":"truncat'
exit 0
