#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

ruby -e '
  failures = []
  files = Dir.glob(File.join(ARGV.fetch(0), ".github/workflows/*.{yml,yaml}"))
  abort "workflow contract: no workflow files found" if files.empty?

  uses_pattern = /^\s*-\s+uses:\s+([^\s#]+)/
  files.each do |path|
    File.readlines(path).each_with_index do |line, index|
      match = uses_pattern.match(line)
      next unless match
      reference = match[1]
      next if reference.start_with?("./")
      next if reference.match?(/\A[^@]+@[0-9a-f]{40}\z/)
      failures << "#{path}:#{index + 1}: action reference is not pinned to a full commit SHA: #{reference}"
    end
  end

  abort failures.join("\n") unless failures.empty?
  puts "workflow contract: #{files.length} workflow file(s), all external actions commit-pinned"
' "$repo_root"
