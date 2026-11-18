#!/bin/bash
# Host runner daemons can deny /proc inspection even for the same user. Keep
# the fail-closed reconciliation guard and test it in a dedicated PID
# namespace, as in the offline local validation. Idle-process assertions also
# need other test packages/scripts not to spawn or replace processes during the
# /proc snapshot. Explicit concurrency tests still exercise their own workers
# and goroutines. The CLI scripts launch hundreds of race-instrumented
# subprocesses; allow their serial run 13 minutes within the 15-minute limit.
set -euo pipefail

go mod download
toolchain_version=$(go env GOVERSION)
test_image="golang:${toolchain_version#go}-bookworm"
docker pull "$test_image"
docker run --rm --pull=never --network none --read-only --user 65534:65534 \
  --memory 3g --memory-swap 3g --cpus 2 --pids-limit 256 \
  --cap-drop ALL --security-opt no-new-privileges \
  --ulimit nofile=1024:1024 --ulimit fsize=1073741824:1073741824 --ulimit cpu=900:900 \
  --tmpfs /scratch:rw,exec,size=2g,mode=1777 \
  --tmpfs /scratch/otherfs:rw,size=32m,mode=1777 --tmpfs /dev/shm:ro,size=16m \
  --mount "type=bind,src=$PWD,dst=/src,readonly" \
  --mount "type=bind,src=$(go env GOMODCACHE),dst=/deps,readonly" \
  --workdir /src --entrypoint /usr/bin/env "$test_image" \
  -i PATH=/usr/local/go/bin:/usr/bin:/bin HOME=/scratch/home TMPDIR=/scratch \
  GOCACHE=/scratch/cache GOMODCACHE=/deps GOPATH=/scratch/gopath \
  GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOTELEMETRY=off CGO_ENABLED=1 GOMAXPROCS=2 \
  AIMS_TEST_SECOND_FS=/scratch/otherfs GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
  GIT_TERMINAL_PROMPT=0 GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0=/src \
  timeout 900 go test -race -count=1 -timeout=13m -p 1 -parallel 1 ./...
