#!/usr/bin/env bash
# Publish docs/wiki/* to GitHub Wiki (vincent1986/AIGateway)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/docs/wiki"
REMOTE_SSH="git@github.com:vincent1986/AIGateway.wiki.git"
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
if [[ -z "$TOKEN" ]] && command -v gh >/dev/null 2>&1; then
  TOKEN="$(gh auth token 2>/dev/null || true)"
fi
if [[ -n "$TOKEN" ]]; then
  REMOTE="https://x-access-token:${TOKEN}@github.com/vincent1986/AIGateway.wiki.git"
else
  REMOTE="$REMOTE_SSH"
fi

if [[ ! -d "$SRC" ]]; then
  echo "missing $SRC" >&2
  exit 1
fi

# Wiki must already be initialized (at least one page created on GitHub).
if ! git ls-remote "$REMOTE" HEAD &>/dev/null; then
  echo "Wiki git repo not found. Create the first page once in the browser:" >&2
  echo "  https://github.com/vincent1986/AIGateway/wiki/_new" >&2
  echo "Title: Home  |  body: (anything)  |  Save Page" >&2
  echo "Then re-run: ./scripts/publish-wiki.sh" >&2
  exit 2
fi

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

git clone "$REMOTE" "$TMP/wiki"
# replace all tracked wiki pages with our source (keep .git)
find "$TMP/wiki" -maxdepth 1 -type f -name '*.md' -delete
cp "$SRC"/*.md "$TMP/wiki/"
cd "$TMP/wiki"
git add -A
if git diff --cached --quiet; then
  echo "Wiki already up to date."
  exit 0
fi
git -c user.email="vincent1986@users.noreply.github.com" -c user.name="vincent1986" \
  commit -m "docs: sync project FAQ wiki from docs/wiki"
# Prefer master (classic wiki), fall back to main
branch="$(git rev-parse --abbrev-ref HEAD)"
git push origin "HEAD:${branch}"
echo "Published to https://github.com/vincent1986/AIGateway/wiki"
