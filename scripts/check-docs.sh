#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

if LC_ALL=C grep -RIn '[^ -~	]' \
    --include='*.md' \
    --include='*.json' \
    --include='*.yaml' \
    --include='*.yml' \
    --exclude-dir='.git' \
    --exclude-dir='graphify-out' \
    .; then
    echo "Documentation and contracts must use ASCII unless explicitly justified." >&2
    exit 1
fi

ruby -e '
files = Dir["**/*.md"].reject { |path| path.start_with?("graphify-out/") }
files.each do |file|
  text = File.read(file)
  abort "#{file}: unbalanced code fences" unless text.scan(/^```/).length.even?
  text.scan(/\[[^\]]+\]\(([^)]+)\)/).flatten.each do |target|
    next if target.start_with?("http://", "https://", "#")
    local = target.split("#", 2).first
    path = File.expand_path(local, File.dirname(file))
    abort "#{file}: missing local link #{target}" unless File.exist?(path)
  end
end
puts "documentation: #{files.length} Markdown files validated"
'

ruby -rjson -e '
files = Dir["api/**/*.json"]
files.each { |file| JSON.parse(File.read(file)) }
puts "contracts: #{files.length} JSON files validated"
'
