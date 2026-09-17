#!/usr/bin/env bash
# Wrapper — see deploy/vm/bootstrap.sh
exec "$(dirname "$0")/../vm/bootstrap.sh" "$@"
