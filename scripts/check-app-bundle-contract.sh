#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
plist="$repo_root/packaging/macos/Info.plist"
route="$repo_root/packaging/tachyon/server/routes/v1/health/yon.sluice"

test -f "$plist"
test -f "$route"
test -s "$route"

ruby -rrexml/document -e '
  document = REXML::Document.new(File.read(ARGV.fetch(0)))
  dictionary = document.elements["plist/dict"] or abort "app bundle contract: missing plist dictionary"
  entries = {}
  children = dictionary.elements.to_a
  children.each_with_index do |element, index|
    next unless element.name == "key"
    entries[element.text] = children.fetch(index + 1)
  end
  expected_strings = {
    "CFBundleDisplayName" => "Sluice",
    "CFBundleExecutable" => "sluice-desktop",
    "CFBundleIdentifier" => "ma.del.sluice.desktop",
    "CFBundlePackageType" => "APPL"
  }
  expected_strings.each do |key, value|
    element = entries[key] or abort "app bundle contract: missing #{key}"
    abort "app bundle contract: #{key} mismatch" unless element.name == "string" && element.text == value
  end
  %w[LSUIElement NSHighResolutionCapable].each do |key|
    element = entries[key] or abort "app bundle contract: missing #{key}"
    abort "app bundle contract: #{key} must be true" unless element.name == "true"
  end
  puts "app bundle contract: Info.plist and Tachyon health route passed"
' "$plist"
