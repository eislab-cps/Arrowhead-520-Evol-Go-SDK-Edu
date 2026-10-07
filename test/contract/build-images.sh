#!/usr/bin/env bash
# Builds the six Arrowhead-520-Go-Evol images the contract test runs against,
# under the names they are published with: ghcr.io/ulfbod/<name>:<TAG>.
#
# Needs:
#   GOEVOL_DIR  the stack source tree at the pinned tag (read only)
#   STACK_REV   the stack revision that tree was taken from. For a git checkout this is
#               the full commit SHA of the tag, the value the stack's images.yml writes
#               as the revision label (github.sha), so local and published images carry
#               the same label.
#   TAG         image tag (default v0.1.3, the tag pinned in CONTRACT.md)
#
# GOEVOL_DIR is either a clean git checkout whose HEAD is exactly TAG, or an
# exported tree carrying a .stack-commit file whose content equals STACK_REV.
# An image that already carries the label revision=STACK_REV is not rebuilt.
# The build list mirrors the stack's .github/workflows/images.yml.
set -euo pipefail

: "${GOEVOL_DIR:?set GOEVOL_DIR to the stack source tree}"
: "${STACK_REV:?set STACK_REV to the stack revision of GOEVOL_DIR}"
TAG="${TAG:-v0.1.3}"

if git -C "$GOEVOL_DIR" rev-parse --git-dir >/dev/null 2>&1; then
  got="$(git -C "$GOEVOL_DIR" describe --tags --exact-match 2>/dev/null || true)"
  [ "$got" = "$TAG" ] || { echo "build-images: $GOEVOL_DIR is at '${got:-no tag}', want $TAG" >&2; exit 1; }
  [ -z "$(git -C "$GOEVOL_DIR" status --porcelain)" ] || { echo "build-images: $GOEVOL_DIR has local changes" >&2; exit 1; }
  head="$(git -C "$GOEVOL_DIR" rev-parse HEAD)"
  [ "$head" = "$STACK_REV" ] || { echo "build-images: STACK_REV must be the full commit $head of $TAG (the published revision label), got $STACK_REV" >&2; exit 1; }
else
  got="$(cat "$GOEVOL_DIR/.stack-commit" 2>/dev/null || true)"
  [ "$got" = "$STACK_REV" ] || { echo "build-images: $GOEVOL_DIR/.stack-commit is '${got:-missing}', want $STACK_REV" >&2; exit 1; }
fi
echo "build-images: source $GOEVOL_DIR at $STACK_REV, tag $TAG"

# name|context|dockerfile|build-arg
IMAGES=(
  "serviceregistry|foundation|deploy/dockerfiles/foundation.Dockerfile|CMD=serviceregistry"
  "authentication|foundation|deploy/dockerfiles/foundation.Dockerfile|CMD=authentication"
  "consumerauth|foundation|deploy/dockerfiles/foundation.Dockerfile|CMD=consumerauth"
  "dynamicorch-xacml|.|deploy/dockerfiles/dynamicorch-xacml.Dockerfile|"
  "profile-ca|.|deploy/dockerfiles/profile-ca.Dockerfile|"
  "cert-provisioner|.|deploy/dockerfiles/cert-provisioner.Dockerfile|"
)

# Build through docker compose, which uses BuildKit via the daemon even where the
# buildx CLI plugin is missing. The stack Dockerfiles need BuildKit since they pin
# the builder and cross-compile (FROM --platform=$BUILDPLATFORM, TARGETOS/TARGETARCH).
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
SPEC="$BUILD_DIR/build.yml"
printf 'name: ah-sdk-imagebuild\nservices:\n' > "$SPEC"
todo=0
for entry in "${IMAGES[@]}"; do
  IFS='|' read -r name context dockerfile buildarg <<<"$entry"
  image="ghcr.io/ulfbod/$name:$TAG"
  have="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$image" 2>/dev/null || true)"
  if [ "$have" = "$STACK_REV" ]; then
    echo "build-images: $image already built from $STACK_REV"
    continue
  fi
  echo "build-images: building $image"
  todo=$((todo + 1))
  {
    printf '  %s:\n    image: "%s"\n    build:\n' "$name" "$image"
    printf '      context: "%s"\n      dockerfile: "%s"\n' "$GOEVOL_DIR/$context" "$GOEVOL_DIR/$dockerfile"
    printf '      labels:\n        org.opencontainers.image.version: "%s"\n        org.opencontainers.image.revision: "%s"\n' "$TAG" "$STACK_REV"
    if [ -n "$buildarg" ]; then
      printf '      args:\n        %s: "%s"\n' "${buildarg%%=*}" "${buildarg#*=}"
    fi
  } >> "$SPEC"
done
if [ "$todo" -gt 0 ]; then
  docker compose -f "$SPEC" build --quiet
fi
echo "build-images: done"
