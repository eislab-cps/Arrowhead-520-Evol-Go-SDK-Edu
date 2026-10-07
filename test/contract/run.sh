#!/usr/bin/env bash
# Tier 3: the contract test. Builds (or reuses) the six stack images, starts
# the harness compose, runs every CONTRACT.md scenario, and always removes the
# stack again with down -v. Exit code is the go test exit code.
#
# Needs GOEVOL_DIR and STACK_REV (see build-images.sh) and Docker.
#   GOEVOL_DIR=... STACK_REV=... bash test/contract/run.sh [go test -run pattern]
#
# PULL_ONLY=1 runs against the published images instead: no local build,
# `docker compose pull` before up, and T0 proves every image is the one the
# registry serves for $TAG (image ID = registry manifest digest) with revision
# label STACK_REV (the full public commit). This is the run that matches what
# students pull.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
: "${GOEVOL_DIR:?set GOEVOL_DIR to the stack source tree}"
: "${STACK_REV:?set STACK_REV to the stack revision of GOEVOL_DIR}"
export TAG="${TAG:-v0.1.3}"
export SYSOP_PASSWORD="${SYSOP_PASSWORD:-$(od -An -tx1 -N12 /dev/urandom | tr -d ' \n')}"
export PULL_ONLY="${PULL_ONLY:-}"
export GOEVOL_DIR STACK_REV

COMPOSE=(docker compose -f "$HERE/docker-compose.yml")
if [ "$PULL_ONLY" = 1 ]; then
  echo "run: PULL_ONLY=1, pulling images, no local build"
  "${COMPOSE[@]}" pull --quiet
else
  bash "$HERE/build-images.sh"
fi
cleanup() { "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; echo "run: stack removed (down -v)"; }
trap cleanup EXIT

"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
start=$(date +%s)
"${COMPOSE[@]}" up -d

# Wait until the five long-running stack services are healthy, mosquitto runs,
# and cert-provisioner has exited 0.
deadline=$((start + 180))
while :; do
  ok=1
  for svc in profile-ca serviceregistry authentication consumerauth dynamicorch-xacml; do
    id="$("${COMPOSE[@]}" ps -q "$svc")"
    [ -n "$id" ] && [ "$(docker inspect -f '{{.State.Health.Status}}' "$id" 2>/dev/null)" = healthy ] || ok=0
  done
  id="$("${COMPOSE[@]}" ps -q mosquitto)"
  [ -n "$id" ] && [ "$(docker inspect -f '{{.State.Running}}' "$id")" = true ] || ok=0
  id="$("${COMPOSE[@]}" ps -a -q cert-provisioner)"
  [ -n "$id" ] && [ "$(docker inspect -f '{{.State.Status}} {{.State.ExitCode}}' "$id")" = "exited 0" ] || ok=0
  [ "$ok" = 1 ] && break
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "run: stack not healthy after 180 s" >&2
    "${COMPOSE[@]}" ps -a >&2
    exit 1
  fi
  sleep 2
done
echo "run: stack healthy after $(( $(date +%s) - start )) s"

export SR_URL=https://localhost:8490 AUTH_URL=https://localhost:8491 CAUTH_URL=https://localhost:8492 \
       ORCH_URL=http://localhost:8083 PCA_HTTP_URL=http://localhost:8787 PCA_TLS_URL=https://localhost:8788 \
       MQTT_URL=tcp://localhost:1883 \
       HARNESS_COMPOSE="$HERE/docker-compose.yml" \
       STACK_COMPOSE="$GOEVOL_DIR/deploy/docker-compose.consumerauth.yml"

cd "$ROOT"
set +e
go test -tags=contract -count=1 -v ${1:+-run "$1"} ./test/contract/...
rc=$?
set -e
exit $rc
