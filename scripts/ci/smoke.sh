#!/usr/bin/env bash
# Post-merge smoke check. Called by Witness after a successful merge.
# Fails fast: build + guard only. Does not run full test suite.
exec "$(dirname "$0")/verify.sh" smoke "$@"
