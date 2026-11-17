#!/usr/bin/env bash
# PreToolUse (Bash): block shell commands that name a .env file, so secrets are
# never read, printed or staged. .env.example is allowed.
# Exit code 2 blocks the command and shows the message to Claude.
input=$(cat)
cmd=$(printf '%s' "$input" | sed 's/\.env\.example//g')
if printf '%s' "$cmd" | grep -qE '(^|[^A-Za-z0-9_])\.env([^A-Za-z0-9_]|$)'; then
  echo "Blocked: this command names a .env file, which holds secrets (CLAUDE.md, Secrets). Use .env.example for variable names, or ask the owner. If .env only appears in text you're writing to a file, use the Write or Edit tool instead." >&2
  exit 2
fi
exit 0
