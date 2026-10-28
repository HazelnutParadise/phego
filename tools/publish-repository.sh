#!/usr/bin/env bash
# Publish only the prepared documentation to a new or empty target repository.
set -euo pipefail

TARGET="HazelnutParadise/phego"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
VISIBILITY="${1:-}"
if [[ "$VISIBILITY" != "--public" && "$VISIBILITY" != "--private" ]]; then
    printf 'Usage: %s --public|--private\n' "$0" >&2
    exit 2
fi
for tool in git gh python3; do
    command -v "$tool" >/dev/null 2>&1 || { printf 'Missing required tool: %s\n' "$tool" >&2; exit 1; }
done
cd "$ROOT"
python3 tools/validate_docs.py
# Authentication belongs to the user's local GitHub CLI; never print tokens.
gh auth status --hostname github.com >/dev/null

if [[ ! -d .git ]]; then
    git init -b main
fi
if [[ "$(git symbolic-ref --quiet --short HEAD)" != "main" ]]; then
    printf 'Refusing to publish a branch other than main.\n' >&2
    exit 1
fi
if ! git rev-parse --verify HEAD >/dev/null 2>&1; then
    git var GIT_AUTHOR_IDENT >/dev/null
    git add -- .gitignore .gitattributes README.md LICENSE AGENTS.md CHANGELOG.md \
        THIRD_PARTY_NOTICES.md PUBLISHING.md docs tools
    git commit -m "docs: establish Phego specification 1.1 and MIT project identity"
fi
if [[ -n "$(git status --porcelain)" ]]; then
    printf 'Refusing to publish a dirty working tree. Review and commit the changes first.\n' >&2
    exit 1
fi

# A failed lookup is not assumed to prove absence: create will refuse an existing
# repository or insufficient privileges without overwriting anything.
if ! gh repo view "$TARGET" --json nameWithOwner >/dev/null 2>&1; then
    gh repo create "$TARGET" "$VISIBILITY" \
        --description "The safest PHP in the world. A security-first PHP engine designed for Go."
fi

URL="https://github.com/${TARGET}.git"
if git remote get-url origin >/dev/null 2>&1; then
    ORIGIN="$(git remote get-url origin)"
    case "$ORIGIN" in
        "$URL"|"https://github.com/${TARGET}"|"git@github.com:${TARGET}.git") ;;
        *) printf 'Refusing unexpected origin: %s\n' "$ORIGIN" >&2; exit 1 ;;
    esac
else
    git remote add origin "$URL"
fi
# Never merge, replace, or force-push an existing remote history automatically.
if [[ -n "$(git ls-remote origin)" ]]; then
    printf 'Target has existing refs. Stop and reconcile its history before pushing.\n' >&2
    exit 1
fi
git push --set-upstream origin HEAD:refs/heads/main
LOCAL_SHA="$(git rev-parse HEAD)"
REMOTE_SHA="$(git ls-remote origin refs/heads/main | awk '{print $1}')"
if [[ "$LOCAL_SHA" != "$REMOTE_SHA" ]]; then
    printf 'Remote verification failed; publication is not confirmed.\n' >&2
    exit 1
fi
printf 'Verified documentation commit %s at %s\n' "$LOCAL_SHA" "$TARGET"
