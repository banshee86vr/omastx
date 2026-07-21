#!/usr/bin/env bash
# Copy article PNGs into lucabertelli.consulting as png/webp/avif.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SRC="$ROOT/docs/screenshots/article"
DEST="${ARTICLE_DEST:-$ROOT/../lucabertelli.consulting/public/blog/omastx}"

if [[ ! -d "$DEST" ]]; then
  echo "Destination not found: $DEST" >&2
  echo "Set ARTICLE_DEST to the consulting public/blog/omastx directory." >&2
  exit 1
fi

map_src_to_dest() {
  case "$1" in
    01-fleet-overview.png) echo fleet ;;
    02-cluster-prod-eu-drift.png) echo cluster_drift ;;
    04-artifacts-ledger.png) echo artifacts ;;
    05-artifacts-filtered.png) echo artifacts_filtered ;;
    07-connect-cluster.png) echo connect ;;
    08-artifact-detail.png) echo artifact_detail ;;
    09-cluster-prod-us-degraded.png) echo cluster_degraded ;;
    *) echo "" ;;
  esac
}

for src in \
  01-fleet-overview.png \
  02-cluster-prod-eu-drift.png \
  04-artifacts-ledger.png \
  05-artifacts-filtered.png \
  07-connect-cluster.png \
  08-artifact-detail.png \
  09-cluster-prod-us-degraded.png
do
  base="$(map_src_to_dest "$src")"
  [[ -n "$base" ]] || continue
  png="$SRC/$src"
  [[ -f "$png" ]] || { echo "missing $png" >&2; exit 1; }

  cp "$png" "$DEST/${base}.png"
  cwebp -quiet -q 82 "$png" -o "$DEST/${base}.webp"
  # AVIF via magick (libheif) when available; otherwise avifenc from PNG.
  if magick "$png" -quality 55 "$DEST/${base}.avif" 2>/dev/null; then
    :
  else
    avifenc --min 0 --max 28 "$png" "$DEST/${base}.avif" >/dev/null
  fi
  echo "synced $base.{png,webp,avif}"
done

echo "Article assets updated in $DEST"
