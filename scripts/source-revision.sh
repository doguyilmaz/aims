#!/bin/sh
set -eu

repo=${1:?repository path required}
if ! revision=$(git -C "$repo" rev-parse --short=12 HEAD 2>/dev/null); then
  printf '%s\n' development
  exit 0
fi
if worktree_status=$(git -C "$repo" status --porcelain --untracked-files=all 2>/dev/null); then
  if [ -n "$worktree_status" ]; then revision="${revision}-dirty"; fi
else
  revision="${revision}-unknown"
fi
printf '%s\n' "$revision"
