#!/bin/sh
# Writes shell completions into completions/ for the release archives.
set -eu
rm -rf completions
mkdir -p completions
for sh in bash zsh fish; do
	go run ./cmd/aims completion "$sh" > "completions/aims.$sh"
done
