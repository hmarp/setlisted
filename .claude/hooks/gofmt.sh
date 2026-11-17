#!/usr/bin/env bash
# PostToolUse (Write|Edit): run gofmt -w on the edited file if it's a .go file.
# Reads the hook's JSON from stdin. No jq needed (works in Git Bash and Linux).
input=$(cat)
f=$(printf '%s' "$input" | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
f=${f//\\\\/\\} # JSON-escaped backslashes in Windows paths
case "$f" in
  *.go) [ -f "$f" ] && gofmt -w "$f" ;;
esac
exit 0
