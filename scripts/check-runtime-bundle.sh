#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
manifest=${1:-"$repo_root/packaging/runtime/manifest.json"}

ruby -rjson -rdigest -rpathname -ruri -e '
  def fail_contract(message)
    abort "runtime bundle contract: #{message}"
  end

  def require_keys(object, expected, context)
    actual = object.keys.sort
    wanted = expected.sort
    fail_contract("#{context} keys #{actual.inspect}, expected #{wanted.inspect}") unless actual == wanted
  end

  def safe_relative_path(value, context)
    fail_contract("#{context} must be a non-empty string") unless value.is_a?(String) && !value.empty?
    path = Pathname.new(value)
    fail_contract("#{context} must be relative") if path.absolute?
    fail_contract("#{context} is not canonical") unless path.cleanpath.to_s == value
    fail_contract("#{context} contains a parent segment") if path.each_filename.any? { |part| part == ".." }
  end

  manifest_path = File.expand_path(ARGV.fetch(0))
  repo_root = File.realpath(ARGV.fetch(1))
  data = JSON.parse(File.read(manifest_path))
  require_keys(data, %w[releaseVersion schemaVersion sources targets], "manifest")
  fail_contract("schemaVersion must be 1") unless data["schemaVersion"] == 1

  version = data["releaseVersion"]
  fail_contract("releaseVersion must be numeric dotted form") unless version.is_a?(String) && version.match?(/\A\d+\.\d+\.\d+\z/)
  sources = data["sources"]
  fail_contract("sources must contain tachyon and fylo") unless sources.is_a?(Hash) && sources.keys.sort == %w[fylo tachyon]

  source_repositories = {
    "tachyon" => "https://github.com/d31ma/Tachyon",
    "fylo" => "https://github.com/d31ma/FYLO"
  }
  source_names = {"tachyon" => "Tachyon", "fylo" => "Fylo"}

  sources.each do |key, source|
    require_keys(source, %w[license name notices releaseUrl repositoryUrl sourceCommit], "source #{key}")
    repository = source_repositories.fetch(key)
    fail_contract("#{key} display name changed") unless source["name"] == source_names.fetch(key)
    fail_contract("#{key} repository changed") unless source["repositoryUrl"] == repository
    fail_contract("#{key} release URL is not version-pinned") unless source["releaseUrl"] == "#{repository}/releases/tag/v#{version}"
    fail_contract("#{key} source commit is not a full SHA") unless source["sourceCommit"].is_a?(String) && source["sourceCommit"].match?(/\A[0-9a-f]{40}\z/)
    fail_contract("#{key} license must be MIT") unless source["license"] == "MIT"
    fail_contract("#{key} must bundle at least one notice") unless source["notices"].is_a?(Array) && !source["notices"].empty?

    source["notices"].each_with_index do |notice, index|
      context = "#{key} notice #{index}"
      require_keys(notice, %w[sha256 sourcePath sourceUrl], context)
      safe_relative_path(notice["sourcePath"], "#{context} sourcePath")
      notice_path = File.expand_path(notice["sourcePath"], repo_root)
      fail_contract("#{context} escapes the repository") unless notice_path.start_with?("#{repo_root}/")
      fail_contract("#{context} is missing #{notice["sourcePath"]}") unless File.file?(notice_path)
      fail_contract("#{context} resolves outside the repository") unless File.realpath(notice_path).start_with?("#{repo_root}/")
      digest = Digest::SHA256.file(notice_path).hexdigest
      fail_contract("#{context} checksum mismatch") unless digest == notice["sha256"]
      expected_url = "https://raw.githubusercontent.com/#{URI(repository).path.delete_prefix("/")}/#{source["sourceCommit"]}/#{File.basename(notice["sourcePath"])}"
      fail_contract("#{context} source URL is not commit-pinned") unless notice["sourceUrl"] == expected_url
      fail_contract("#{context} source URL must not contain query or fragment") unless URI(notice["sourceUrl"]).query.nil? && URI(notice["sourceUrl"]).fragment.nil?
    end
  end

  targets = data["targets"]
  fail_contract("targets must be a non-empty array") unless targets.is_a?(Array) && !targets.empty?
  expected_targets = {
    "macos-arm64" => ["darwin", "arm64"],
    "macos-amd64" => ["darwin", "amd64"],
    "linux-arm64" => ["linux", "arm64"],
    "linux-amd64" => ["linux", "amd64"],
    "windows-amd64" => ["windows", "amd64"]
  }
  expected_assets = {
    "macos-arm64" => {"tachyon" => "ty-macos-arm64", "fylo" => "fylo-macos-arm64"},
    "macos-amd64" => {"tachyon" => "ty-macos-x64", "fylo" => "fylo-macos-x64"},
    "linux-arm64" => {"tachyon" => "ty-linux-arm64", "fylo" => "fylo-linux-arm64"},
    "linux-amd64" => {"tachyon" => "ty-linux-x64", "fylo" => "fylo-linux-x64"},
    "windows-amd64" => {"tachyon" => "ty-windows-x64.exe", "fylo" => "fylo-windows-x64.exe"}
  }
  fail_contract("target matrix changed") unless targets.map { |target| target["id"] }.sort == expected_targets.keys.sort

  targets.each do |target|
    require_keys(target, %w[arch artifacts evidence id noticeDirectory os qualification], "target")
    expected_os, expected_arch = expected_targets.fetch(target["id"])
    fail_contract("#{target["id"]} OS/architecture mismatch") unless target["os"] == expected_os && target["arch"] == expected_arch
    allowed_qualifications = %w[native-staging-pending native-staging-passed not-native-tested]
    fail_contract("#{target["id"]} has unknown qualification") unless allowed_qualifications.include?(target["qualification"])
    if target["id"] != "macos-arm64" && target["qualification"] != "not-native-tested"
      fail_contract("#{target["id"]} has no native staging evidence")
    end
    if target["qualification"] == "native-staging-passed"
      safe_relative_path(target["evidence"], "#{target["id"]} evidence")
      fail_contract("#{target["id"]} evidence must be a runtime qualification record") unless target["evidence"].start_with?("docs/qualification/runtime/")
      evidence_path = File.expand_path(target["evidence"], repo_root)
      fail_contract("#{target["id"]} evidence is missing") unless File.file?(evidence_path)
    elsif !target["evidence"].nil?
      fail_contract("#{target["id"]} must not cite evidence before a native staging pass")
    end
    safe_relative_path(target["noticeDirectory"], "#{target["id"]} noticeDirectory")

    artifacts = target["artifacts"]
    fail_contract("#{target["id"]} must contain exactly two runtime artifacts") unless artifacts.is_a?(Array) && artifacts.length == 2
    fail_contract("#{target["id"]} must contain Tachyon and Fylo") unless artifacts.map { |artifact| artifact["runtime"] }.sort == %w[fylo tachyon]
    install_paths = []
    artifacts.each do |artifact|
      require_keys(artifact, %w[asset downloadUrl installRelativePath runtime sha256], "#{target["id"]} artifact")
      runtime = artifact["runtime"]
      repository = source_repositories.fetch(runtime)
      fail_contract("#{target["id"]} #{runtime} asset does not match the target") unless artifact["asset"] == expected_assets.fetch(target["id"]).fetch(runtime)
      expected_url = "#{repository}/releases/download/v#{version}/#{artifact["asset"]}"
      fail_contract("#{target["id"]} #{runtime} download URL is not the exact release asset") unless artifact["downloadUrl"] == expected_url
      fail_contract("#{target["id"]} #{runtime} has an invalid SHA-256") unless artifact["sha256"].is_a?(String) && artifact["sha256"].match?(/\A[0-9a-f]{64}\z/)
      safe_relative_path(artifact["installRelativePath"], "#{target["id"]} #{runtime} installRelativePath")
      install_paths << artifact["installRelativePath"]
    end
    fail_contract("#{target["id"]} artifacts collide") unless install_paths.uniq.length == install_paths.length
  end

  serialized = File.read(manifest_path)
  fail_contract("mutable download reference found") if serialized.match?(%r{/(latest|main|master)(/|\z)})
  puts "runtime bundle contract: #{targets.length} targets, #{targets.sum { |target| target["artifacts"].length }} checksummed artifacts"
' "$manifest" "$repo_root"
