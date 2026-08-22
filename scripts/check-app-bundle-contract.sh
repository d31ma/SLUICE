#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
plist="$repo_root/packaging/macos/Info.plist"
route="$repo_root/packaging/tachyon/server/routes/v1/health/yon.sluice"
desktop_entry="$repo_root/packaging/linux/sluice.desktop"
windows_manifest="$repo_root/packaging/windows/app-manifest.json"

test -f "$plist"
test -f "$desktop_entry"
test -f "$windows_manifest"
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

ruby -e '
  entries = File.readlines(ARGV.fetch(0), chomp: true).filter_map do |line|
    next if line.empty? || line.start_with?("[")
    key, value = line.split("=", 2)
    [key, value]
  end.to_h
  expected = {
    "Type" => "Application",
    "Name" => "Sluice",
    "Exec" => "AppRun",
    "Terminal" => "false"
  }
  expected.each do |key, value|
    abort "app bundle contract: Linux #{key} mismatch" unless entries[key] == value
  end
  puts "app bundle contract: Linux desktop entry passed"
' "$desktop_entry"

ruby -rjson -e '
  manifest = JSON.parse(File.read(ARGV.fetch(0)))
  expected = {
    "schemaVersion" => 1,
    "applicationId" => "ma.del.sluice.desktop",
    "name" => "Sluice",
    "entrypoint" => "sluice-desktop.exe",
    "architecture" => "amd64",
    "runtimeVersion" => "26.33.02"
  }
  abort "app bundle contract: Windows manifest mismatch" unless manifest == expected
  puts "app bundle contract: Windows package manifest passed"
' "$windows_manifest"
