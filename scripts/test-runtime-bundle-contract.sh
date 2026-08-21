#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
checker="$repo_root/scripts/check-runtime-bundle.sh"
manifest="$repo_root/packaging/runtime/manifest.json"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/sluice-runtime-contract.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

"$checker" "$manifest"

ruby -rjson -e '
  data = JSON.parse(File.read(ARGV.fetch(0)))
  data["targets"][0]["artifacts"][0]["downloadUrl"] = "https://github.com/d31ma/Tachyon/releases/latest/download/ty-macos-arm64"
  File.write(ARGV.fetch(1), JSON.pretty_generate(data))
' "$manifest" "$test_root/mutable.json"
if "$checker" "$test_root/mutable.json" >/dev/null 2>&1; then
  echo "runtime bundle test: mutable URL was accepted" >&2
  exit 1
fi

ruby -rjson -e '
  data = JSON.parse(File.read(ARGV.fetch(0)))
  data["targets"][0]["artifacts"][0]["installRelativePath"] = "../../usr/local/bin/ty"
  File.write(ARGV.fetch(1), JSON.pretty_generate(data))
' "$manifest" "$test_root/traversal.json"
if "$checker" "$test_root/traversal.json" >/dev/null 2>&1; then
  echo "runtime bundle test: path traversal was accepted" >&2
  exit 1
fi

ruby -rjson -e '
  data = JSON.parse(File.read(ARGV.fetch(0)))
  data["sources"]["fylo"]["notices"][0]["sha256"] = "0" * 64
  File.write(ARGV.fetch(1), JSON.pretty_generate(data))
' "$manifest" "$test_root/bad-notice.json"
if "$checker" "$test_root/bad-notice.json" >/dev/null 2>&1; then
  echo "runtime bundle test: incorrect notice checksum was accepted" >&2
  exit 1
fi

echo "runtime bundle contract tests: passed"
