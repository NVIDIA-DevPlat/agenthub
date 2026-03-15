#!/usr/bin/env bash
# agenthub release script
# Usage: ./scripts/release.sh [major|minor|patch]
# Default: patch bump

set -euo pipefail

BUMP_TYPE="${1:-patch}"

# ── Preflight ──────────────────────────────────────────────────────────────────
if ! command -v gh &>/dev/null; then
    echo "ERROR: GitHub CLI (gh) not found. Install: brew install gh" >&2; exit 1
fi
if ! gh auth status &>/dev/null; then
    echo "ERROR: gh not authenticated. Run: gh auth login" >&2; exit 1
fi
if [[ -n $(git status --porcelain) ]]; then
    echo "ERROR: Working directory is not clean. Commit or stash changes first." >&2; exit 1
fi
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" != "main" ]]; then
    echo "ERROR: Must be on main branch (currently: $BRANCH)" >&2; exit 1
fi

# ── Version arithmetic ─────────────────────────────────────────────────────────
CURRENT=$(cat VERSION 2>/dev/null || echo "0.0.0")
IFS='.' read -r MAJ MIN PAT <<< "$CURRENT"
case "$BUMP_TYPE" in
    major) MAJ=$((MAJ + 1)); MIN=0; PAT=0 ;;
    minor) MIN=$((MIN + 1)); PAT=0 ;;
    patch) PAT=$((PAT + 1)) ;;
    *) echo "ERROR: invalid bump type '$BUMP_TYPE' (use major|minor|patch)" >&2; exit 1 ;;
esac
NEXT="$MAJ.$MIN.$PAT"

echo "Releasing: $CURRENT → $NEXT"
read -rp "Proceed? [y/N] " confirm
[[ "$confirm" =~ ^[Yy]$ ]] || { echo "Cancelled."; exit 0; }

# ── Bump VERSION and commit ────────────────────────────────────────────────────
echo "$NEXT" > VERSION
git add VERSION
git commit -m "chore: release v$NEXT

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"

# ── Tag and push ───────────────────────────────────────────────────────────────
git tag -a "v$NEXT" -m "Release v$NEXT"
git push origin main
git push origin "v$NEXT"

echo ""
echo "Tag v$NEXT pushed. GitHub Actions will build the Linux binary."
echo "Monitor: $(gh repo view --json url -q .url)/actions"
echo ""
echo "Release will appear at: $(gh repo view --json url -q .url)/releases/tag/v$NEXT"
