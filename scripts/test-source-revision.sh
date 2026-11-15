#!/bin/bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
helper="$repo/scripts/source-revision.sh"
scratch=$(mktemp -d "${TMPDIR:-/tmp}/aims-revision-test.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0
fixture="$scratch/repo"
git -c init.defaultBranch=main init --quiet "$fixture"
printf 'dist/\n' > "$fixture/.gitignore"
printf 'let value = 1\n' > "$fixture/Tracked.swift"
git -C "$fixture" add .
git -C "$fixture" -c core.hooksPath=/dev/null -c commit.gpgsign=false \
  -c user.name=Fixture -c user.email=fixture@example.invalid commit --quiet -m fixture
revision=$(git -C "$fixture" rev-parse --short=12 HEAD)

expect_revision() {
  actual=$("$helper" "$fixture")
  if [[ "$actual" != "$1" ]]; then
    echo "Expected $1, got $actual" >&2
    exit 1
  fi
}

expect_revision "$revision"
printf 'let added = 2\n' > "$fixture/Untracked.swift"
expect_revision "${revision}-dirty"
rm "$fixture/Untracked.swift"
mkdir "$fixture/dist"
printf 'ignored build output\n' > "$fixture/dist/output"
expect_revision "$revision"
printf 'let value = 3\n' > "$fixture/Tracked.swift"
expect_revision "${revision}-dirty"
[[ $("$helper" "$scratch") == development ]]
mkdir "$scratch/bin"
cat > "$scratch/bin/git" <<'EOF'
#!/bin/sh
case "${3:-}" in
  rev-parse) printf '%s\n' 0123456789ab ;;
  status) exit 1 ;;
  *) exit 2 ;;
esac
EOF
chmod +x "$scratch/bin/git"
[[ $(PATH="$scratch/bin:$PATH" "$helper" "$fixture") == 0123456789ab-unknown ]]
echo 'Source revision checks passed: clean, untracked, ignored, modified, non-Git and unreadable status.'
