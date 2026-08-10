# frozen_string_literal: true

require "fileutils"
require "json"
require "minitest/autorun"
require "open3"
require "tmpdir"

PROJECT_ROOT = File.expand_path("../..", __dir__)
MERGER = File.join(PROJECT_ROOT, "ops/merge-sub2api-release.sh")
RESOLVER = File.join(PROJECT_ROOT, "ops/resolve-sub2api-release-conflicts.sh")

BASE_169_COMMIT = "26d894ef4f50645a4bf1030e378ac892f17d0223"
TARGET_171_TAG_OBJECT = "afd154b92aac36c6dafb1fa8e181ca827c78c465"
TARGET_171_COMMIT = "f0e7a9c7a23a7d02fb159b62fa809621eb0475a6"

class MergeSub2APIReleaseTest < Minitest::Test
  def test_allows_a_same_tree_forced_rebuild_audit_commit
    assert_includes File.read(MERGER), 'commit -q --allow-empty -m'
  end

  def test_accepts_a_clean_git_worktree_as_the_release_root
    with_repositories do |fixture|
      worktree = File.join(fixture.fetch(:dir), "release-worktree")
      git(fixture.fetch(:root), "worktree", "add", "-q", "-b", "release-worktree", worktree, fixture.fetch(:root_base))
      fixture = fixture.merge(root: worktree)

      status, output = run_merge(fixture)

      assert status.success?, output
    end
  end

  def test_merges_official_delta_with_custom_snapshot_and_exports_bundle
    with_repositories do |fixture|
      status, output = run_merge(fixture)

      assert status.success?, output
      assert_equal "official\n", File.read(File.join(fixture[:root], "upstream/sub2api/app.txt"))
      assert_equal "custom\n", File.read(File.join(fixture[:root], "upstream/sub2api/custom.txt"))
      assert_equal "new\n", File.read(File.join(fixture[:root], "upstream/sub2api/new.txt"))
      refute Dir.exist?(File.join(fixture[:root], "upstream/sub2api/.git"))
      assert File.file?(fixture[:bundle])
      assert_equal 0o600, File.stat(fixture[:report]).mode & 0o777

      report = JSON.parse(File.read(fixture[:report]))
      assert_match(/\A[0-9a-f]{40}\z/, report.fetch("candidate_commit"))
      assert_equal fixture[:root_base], git(fixture[:root], "rev-parse", "HEAD^").strip

      imported = File.join(fixture[:dir], "imported")
      system("git", "clone", "-q", fixture[:root], imported) or flunk "clone failed"
      system("git", "-C", imported, "fetch", "-q", fixture[:bundle], "candidate-artifact:candidate-artifact") or flunk "bundle import failed"
      assert_equal report.fetch("candidate_commit"), git(imported, "rev-parse", "candidate-artifact").strip
    end
  end

  def test_materializes_target_version_in_candidate_source_before_export
    with_repositories(
      base_version: "0.1.171",
      release_version: "0.1.173",
      release_tag: "v0.1.173",
      source_version: "0.1.172"
    ) do |fixture|
      status, output = run_merge(fixture)

      assert status.success?, output
      version_path = File.join(fixture[:root], "upstream/sub2api/backend/cmd/server/VERSION")
      assert_equal "0.1.173\n", File.binread(version_path)

      report = JSON.parse(File.read(fixture[:report]))
      assert_equal "0.1.173\n", git(
        fixture[:root], "show",
        "#{report.fetch("candidate_commit")}:upstream/sub2api/backend/cmd/server/VERSION"
      )
    end
  end

  def test_fails_closed_when_stale_candidate_version_cannot_be_materialized
    with_repositories(
      base_version: "0.1.171",
      release_version: "0.1.173",
      release_tag: "v0.1.173",
      source_version: "0.1.172",
      source_version_symlink: true
    ) do |fixture|
      before_head = git(fixture[:root], "rev-parse", "HEAD").strip

      status, output = run_merge(fixture)

      refute status.success?, output
      assert_includes output, "sub2api_merge status=failed"
      assert_equal before_head, git(fixture[:root], "rev-parse", "HEAD").strip
      refute File.exist?(fixture[:bundle])
      refute File.exist?(fixture[:report])
    end
  end

  def test_allows_same_version_forced_rebuild_when_version_is_already_materialized
    with_repositories(
      base_version: "0.1.167",
      release_version: "0.1.167",
      release_tag: "v0.1.167",
      source_version: "0.1.167",
      same_official_commit: true
    ) do |fixture|
      status, output = run_merge(fixture)

      assert status.success?, output
      assert File.file?(fixture[:bundle])
      report = JSON.parse(File.read(fixture[:report]))
      assert_equal "0.1.167", report.fetch("version")
      assert_equal fixture[:official_base], report.fetch("official_commit")
    end
  end

  def test_tracks_new_official_files_even_when_the_snapshot_gitignore_matches
    with_repositories(ignored_official_addition: true) do |fixture|
      status, output = run_merge(fixture)

      assert status.success?, output
      tracked_paths = git(fixture[:root], "ls-tree", "-r", "--name-only", "HEAD").lines.map(&:strip)
      assert_includes tracked_paths, "upstream/sub2api/docs/official-safe-defaults.md"
      refute_includes tracked_paths, "upstream/sub2api/docs/local-ignored.md"
      assert_equal "official tracked\n", File.read(
        File.join(fixture[:root], "upstream/sub2api/docs/official-safe-defaults.md")
      )
    end
  end

  def test_conflict_fails_before_changing_root_head_or_files
    with_repositories(conflict: true) do |fixture|
      before_head = git(fixture[:root], "rev-parse", "HEAD")
      before_tree = git(fixture[:root], "status", "--porcelain=v1")

      status, = run_merge(fixture)

      refute status.success?
      assert_equal before_head, git(fixture[:root], "rev-parse", "HEAD")
      assert_equal before_tree, git(fixture[:root], "status", "--porcelain=v1")
      refute File.exist?(fixture[:bundle])
      refute File.exist?(fixture[:report])
    end
  end

  def test_unknown_release_conflict_remains_fail_closed
    with_repositories(conflict: true) do |fixture|
      before_head = git(fixture[:root], "rev-parse", "HEAD")

      status, output = run_merge(fixture)

      refute status.success?
      assert_includes output, "sub2api_merge status=failed"
      assert_equal before_head, git(fixture[:root], "rev-parse", "HEAD")
      refute File.exist?(fixture[:bundle])
      refute File.exist?(fixture[:report])
    end
  end

  def test_v0171_does_not_fallback_to_wire_only_when_resolver_fails
    with_repositories(
      generated_conflict: true,
      base_version: "0.1.169",
      release_version: "0.1.171",
      release_tag: "v0.1.171"
    ) do |fixture|
      status, output = run_merge(fixture)

      refute status.success?
      assert_includes output, "sub2api_release_resolution status=failed reason="
      refute_includes output, "sub2api_merge generated_paths="
      refute File.exist?(fixture[:bundle])
      refute File.exist?(fixture[:report])
    end
  end

  def test_v0173_does_not_fallback_to_wire_only_when_resolver_fails
    with_repositories(
      generated_conflict: true,
      base_version: "0.1.171",
      release_version: "0.1.173",
      release_tag: "v0.1.173"
    ) do |fixture|
      status, output = run_merge(fixture)

      refute status.success?
      assert_includes output, "sub2api_release_resolution status=failed reason="
      refute_includes output, "sub2api_merge generated_paths="
      refute File.exist?(fixture[:bundle])
      refute File.exist?(fixture[:report])
    end
  end

  def test_regenerates_wire_output_when_it_is_the_only_merge_conflict
    with_repositories(generated_conflict: true) do |fixture|
      status, output = run_merge(fixture)

      assert status.success?, output
      wire_output = File.join(
        fixture[:root], "upstream/sub2api/backend/cmd/server/wire_gen.go"
      )
      assert_equal <<~GO, File.read(wire_output)
        // Code generated by fixture. DO NOT EDIT.
        package main

        const generatedValue = "merged"
      GO
    end
  end

  def test_exact_v0171_resolution_applies_recorded_postimage_and_regenerates_generated_files
    with_resolver_fixture do |fixture|
      status, output = run_resolver(fixture)

      assert status.success?, output
      assert_equal "resolved\n", File.read(File.join(fixture[:repository], "semantic.txt"))
      assert_equal "resolved clean\n", File.read(File.join(fixture[:repository], "clean.txt"))
      assert_equal "generated wire\n", File.read(File.join(fixture[:repository], "backend/cmd/server/wire_gen.go"))
      assert_equal "generated sum\n", File.read(File.join(fixture[:repository], "backend/go.sum"))
      assert_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
      assert_empty git(fixture[:repository], "diff", "--name-only")
    end
  end

  def test_v0173_resolution_seeds_ent_generated_tree_before_regeneration
    with_resolver_fixture(
      generation_profile: "ent_and_wire",
      auto_merged_generated: true,
      include_clean_preimage: false
    ) do |fixture|
      status, output = run_resolver(fixture, target_version: "0.1.173", target_tag: "v0.1.173")

      assert status.success?, output
      assert_equal "generated ent\n", File.read(File.join(fixture[:repository], "backend/ent/generated.go"))
      assert_equal "generated group\n", File.read(File.join(fixture[:repository], "backend/ent/group.go"))
      assert_equal "generated wire\n", File.read(File.join(fixture[:repository], "backend/cmd/server/wire_gen.go"))
      assert_equal "seed\n", File.read(File.join(fixture[:repository], "backend/ent/seed.go"))
      assert_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
      assert_empty git(fixture[:repository], "diff", "--name-only")
    end
  end

  def test_v0173_resolution_rejects_generated_content_mismatch_and_restores_merge_state
    with_resolver_fixture(generation_profile: "ent_and_wire", include_clean_preimage: false) do |fixture|
      fake_go = File.join(fixture.fetch(:fake_bin), "go")
      File.write(
        fake_go,
        File.read(fake_go).sub("generated wire\n", "tampered wire\n")
      )

      status, output = run_resolver(fixture, target_version: "0.1.173", target_tag: "v0.1.173")

      refute status.success?
      assert_includes output, "reason=generation_postimage_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0173_resolution_rejects_ent_seed_preimage_mismatch_without_writing
    with_resolver_fixture(generation_profile: "ent_and_wire", include_clean_preimage: false) do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest.fetch("ent_seed_preimages")["backend/ent/seed.go"] = "0" * 40
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

      status, output = run_resolver(fixture, target_version: "0.1.173", target_tag: "v0.1.173")

      refute status.success?
      assert_includes output, "reason=preimage_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0173_resolution_rejects_unsafe_ent_seed_paths_before_writing
    unsafe_paths = [
      "backend/ent/../escape.go",
      "backend/ent/seed.go\tgenerated\tbackend/cmd/server/wire_gen.go",
      "backend/ent/seed.go\nclean\tsemantic.txt\t#{"0" * 40}"
    ]

    unsafe_paths.each do |unsafe_path|
      with_resolver_fixture(generation_profile: "ent_and_wire", include_clean_preimage: false) do |fixture|
        manifest = JSON.parse(File.read(fixture[:manifest]))
        seed_paths = manifest.fetch("ent_seed_paths")
        seed_index = seed_paths.index("backend/ent/seed.go")
        seed_blob = manifest.fetch("ent_seed_preimages").delete("backend/ent/seed.go")
        seed_paths[seed_index] = unsafe_path
        manifest.fetch("ent_seed_preimages")[unsafe_path] = seed_blob
        File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

        status, output = run_resolver(fixture, target_version: "0.1.173", target_tag: "v0.1.173")

        refute status.success?
        assert_includes output, "reason=record_invalid"
        refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
      end
    end
  end

  def test_v0171_resolution_ignores_staged_auto_merged_paths_when_checking_generated_scope
    with_resolver_fixture(auto_merged_path: true) do |fixture|
      status, output = run_resolver(fixture)

      assert status.success?, output
      assert_equal "official auto\n", File.read(File.join(fixture[:repository], "auto.txt"))
      assert_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
      assert_empty git(fixture[:repository], "diff", "--name-only")
    end
  end

  def test_v0171_resolution_rejects_target_identity_mismatch_without_writing
    with_resolver_fixture do |fixture|
      semantic = File.join(fixture[:repository], "semantic.txt")
      before = File.binread(semantic)

      status, output = run_resolver(fixture, target_commit: "f" * 40)

      refute status.success?
      assert_includes output, "reason=target_identity_mismatch"
      assert_equal before, File.binread(semantic)
      assert_equal "base clean\n", File.read(File.join(fixture[:repository], "clean.txt"))
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_conflict_set_mismatch_without_writing
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest.fetch("conflicts").delete("semantic.txt")
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")
      semantic = File.join(fixture[:repository], "semantic.txt")
      before = File.binread(semantic)

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=conflict_set_mismatch"
      assert_equal before, File.binread(semantic)
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_preimage_mismatch_without_writing
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest.fetch("conflicts").fetch("semantic.txt").fetch("stages")["2"] = "0" * 40
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")
      semantic = File.join(fixture[:repository], "semantic.txt")
      before = File.binread(semantic)

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=preimage_mismatch"
      assert_equal before, File.binread(semantic)
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_stage1_preimage_mismatch_without_writing
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest.fetch("conflicts").fetch("semantic.txt").fetch("stages")["1"] = "0" * 40
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=preimage_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_stage3_preimage_mismatch_without_writing
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest.fetch("conflicts").fetch("semantic.txt").fetch("stages")["3"] = "0" * 40
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=preimage_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_tag_object_mismatch_without_writing
    with_resolver_fixture do |fixture|
      git(fixture[:repository], "tag", "-d", "v0.1.171")
      git(fixture[:repository], "tag", "-a", "v0.1.171", "-m", "replacement", fixture[:target_commit])

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=target_identity_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_base_commit_not_present_in_repository
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest["base_commit"] = "f" * 40
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

      status, output = run_resolver(fixture, base_commit: "f" * 40)

      refute status.success?
      assert_includes output, "reason=base_identity_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_target_commit_not_referenced_by_tag
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest["target_commit"] = fixture[:base_commit]
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

      status, output = run_resolver(fixture, target_commit: fixture[:base_commit])

      refute status.success?
      assert_includes output, "reason=target_identity_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_patch_scope_outside_recorded_files
    with_resolver_fixture do |fixture|
      patch = File.join(fixture[:records_root], "0.1.169-to-0.1.171/postimages/semantic.patch")
      File.open(patch, "ab") do |file|
        file.write(<<~PATCH)

          diff --git a/unexpected.txt b/unexpected.txt
          --- a/unexpected.txt
          +++ b/unexpected.txt
          @@ -0,0 +1 @@
          +unexpected
        PATCH
      end
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest["resolution_patch_blob"] = git(fixture[:records_root], "hash-object", patch).strip
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=patch_scope_mismatch"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_rejects_clean_preimage_mismatch_without_writing
    with_resolver_fixture do |fixture|
      manifest = JSON.parse(File.read(fixture[:manifest]))
      manifest.fetch("clean_preimages")["clean.txt"] = "0" * 40
      File.write(fixture[:manifest], JSON.pretty_generate(manifest) + "\n")
      semantic = File.join(fixture[:repository], "semantic.txt")
      before = File.binread(semantic)

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=preimage_mismatch"
      assert_equal before, File.binread(semantic)
      assert_equal "base clean\n", File.read(File.join(fixture[:repository], "clean.txt"))
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_v0171_resolution_restores_original_merge_state_when_generation_fails
    with_resolver_fixture do |fixture|
      File.write(File.join(fixture[:repository], "backend/go.mod"), "module example.invalid/fixture\n\ngo 1.24\n// user change\n")
      File.write(File.join(fixture[:repository], "scratch-generated.txt"), "keep me\n")
      File.write(File.join(fixture.fetch(:fake_bin), "go"), <<~SH)
        #!/usr/bin/env bash
        set -euo pipefail
        [[ "$1" == "-C" ]]
        backend=$2
        shift 2
        case "$*" in
          "mod tidy") printf 'partial generated sum\n' > "$backend/go.sum" ;;
          "generate ./cmd/server") exit 65 ;;
          *) exit 64 ;;
        esac
      SH
      FileUtils.chmod(0o755, File.join(fixture.fetch(:fake_bin), "go"))
      index_path = File.expand_path(git(fixture[:repository], "rev-parse", "--git-path", "index").strip, fixture[:repository])
      before_index = File.binread(index_path)
      before_files = resolver_fixture_files(fixture[:repository]).to_h do |path|
        [path, File.binread(File.join(fixture[:repository], path))]
      end

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=generation_failed"
      assert_equal before_index, File.binread(index_path)
      before_files.each do |path, content|
        assert_equal content, File.binread(File.join(fixture[:repository], path)), path
      end
      assert_equal "module example.invalid/fixture\n\ngo 1.24\n// user change\n", File.read(File.join(fixture[:repository], "backend/go.mod"))
      assert_equal "keep me\n", File.read(File.join(fixture[:repository], "scratch-generated.txt"))
    end
  end

  def test_v0171_resolution_rejects_extra_generated_file_and_restores_worktree
    with_resolver_fixture do |fixture|
      File.write(File.join(fixture.fetch(:fake_bin), "go"), <<~SH)
        #!/usr/bin/env bash
        set -euo pipefail
        [[ "$1" == "-C" ]]
        backend=$2
        shift 2
        case "$*" in
          "mod tidy") printf 'generated sum\n' > "$backend/go.sum" ;;
          "generate ./cmd/server")
            printf 'generated wire\n' > "$backend/cmd/server/wire_gen.go"
            printf 'unexpected\n' > "$backend/generated-extra.txt"
            ;;
          *) exit 64 ;;
        esac
      SH
      FileUtils.chmod(0o755, File.join(fixture.fetch(:fake_bin), "go"))

      status, output = run_resolver(fixture)

      refute status.success?
      assert_includes output, "reason=generation_scope_mismatch"
      refute File.exist?(File.join(fixture[:repository], "backend/generated-extra.txt"))
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  def test_unknown_future_release_has_no_resolution_record
    with_resolver_fixture do |fixture|
      status, output = run_resolver(
        fixture,
        target_version: "0.1.172",
        target_tag: "v0.1.172"
      )

      refute status.success?
      assert_includes output, "reason=record_missing"
      refute_empty git(fixture[:repository], "diff", "--name-only", "--diff-filter=U")
    end
  end

  private

  def with_resolver_fixture(
    auto_merged_path: false,
    generation_profile: "wire_and_modules",
    auto_merged_generated: false,
    include_clean_preimage: true
  )
    Dir.mktmpdir("resolve-sub2api-release") do |dir|
      repository = File.join(dir, "repository")
      records_root = File.join(dir, "records")
      target_version = generation_profile == "ent_and_wire" ? "0.1.173" : "0.1.171"
      target_tag = "v#{target_version}"
      record = File.join(records_root, "0.1.169-to-#{target_version}")
      FileUtils.mkdir_p([
        File.join(repository, "backend/cmd/server"),
        File.join(repository, "backend/ent"),
        File.join(record, "postimages")
      ])
      git(repository, "init", "-q")
      configure_git(repository)

      File.write(File.join(repository, "semantic.txt"), "base\n")
      File.write(File.join(repository, "clean.txt"), "base clean\n")
      File.write(File.join(repository, "auto.txt"), "base auto\n") if auto_merged_path
      File.write(File.join(repository, "backend/go.mod"), "module example.invalid/fixture\n\ngo 1.24\n")
      File.write(File.join(repository, "backend/cmd/server/wire_gen.go"), "base wire\n")
      if generation_profile == "ent_and_wire"
        File.write(File.join(repository, "backend/ent/seed.go"), "seed\n")
        File.write(File.join(repository, "backend/ent/generated.go"), "base ent\n")
        File.write(File.join(repository, "backend/ent/group.go"), "base group\n") if auto_merged_generated
      else
        File.write(File.join(repository, "backend/go.sum"), "base sum\n")
      end
      git(repository, "add", ".")
      git(repository, "commit", "-q", "-m", "base")
      base_commit = git(repository, "rev-parse", "HEAD").strip
      base_branch = git(repository, "branch", "--show-current").strip
      git(repository, "branch", "target")

      File.write(File.join(repository, "semantic.txt"), "ours\n")
      File.write(File.join(repository, "backend/cmd/server/wire_gen.go"), "ours wire\n")
      if generation_profile == "ent_and_wire"
        File.write(File.join(repository, "backend/ent/generated.go"), "ours ent\n")
      else
        File.write(File.join(repository, "backend/go.sum"), "ours sum\n")
      end
      git(repository, "add", ".")
      git(repository, "commit", "-q", "-m", "ours")

      git(repository, "checkout", "-q", "target")
      File.write(File.join(repository, "semantic.txt"), "theirs\n")
      File.write(File.join(repository, "backend/cmd/server/wire_gen.go"), "theirs wire\n")
      if generation_profile == "ent_and_wire"
        File.write(File.join(repository, "backend/ent/generated.go"), "theirs ent\n")
        File.write(File.join(repository, "backend/ent/group.go"), "official group\n") if auto_merged_generated
      else
        File.write(File.join(repository, "backend/go.sum"), "theirs sum\n")
      end
      File.write(File.join(repository, "auto.txt"), "official auto\n") if auto_merged_path
      git(repository, "add", ".")
      git(repository, "commit", "-q", "-m", "theirs")
      target_commit = git(repository, "rev-parse", "HEAD").strip
      git(repository, "tag", "-a", target_tag, "-m", "release", target_commit)
      target_tag_object = git(repository, "rev-parse", target_tag).strip
      git(repository, "checkout", "-q", base_branch)
      _stdout, _stderr, merge_status = Open3.capture3("git", "-C", repository, "merge", "--no-ff", "--no-edit", "target")
      refute merge_status.success?, "fixture merge unexpectedly succeeded"

      semantic_path = File.join(repository, "semantic.txt")
      original_conflict = File.binread(semantic_path)
      git(repository, "checkout", "--conflict=merge", "--", "semantic.txt")
      conflict_lines = File.readlines(semantic_path)
      postimage = File.join(record, "postimages/semantic.patch")
      patch = <<~PATCH
        diff --git a/semantic.txt b/semantic.txt
        --- a/semantic.txt
        +++ b/semantic.txt
        @@ -1,#{conflict_lines.length} +1 @@
        #{conflict_lines.map { |line| "-#{line}" }.join}+resolved
      PATCH
      if include_clean_preimage
        patch += <<~PATCH
          diff --git a/clean.txt b/clean.txt
          --- a/clean.txt
          +++ b/clean.txt
          @@ -1 +1 @@
          -base clean
          +resolved clean
        PATCH
      end
      File.write(postimage, patch)
      File.binwrite(semantic_path, original_conflict)
      conflicts = unmerged_stages(repository).transform_values do |stages|
        { "stages" => stages }
      end
      generated_contents = if generation_profile == "ent_and_wire"
        contents = {
          "backend/cmd/server/wire_gen.go" => "generated wire\n",
          "backend/ent/generated.go" => "generated ent\n"
        }
        contents["backend/ent/group.go"] = "generated group\n" if auto_merged_generated
        contents
      else
        {
          "backend/cmd/server/wire_gen.go" => "generated wire\n",
          "backend/go.sum" => "generated sum\n"
        }
      end
      manifest = File.join(record, "manifest.json")
      manifest_data = {
        "base_version" => "0.1.169",
        "base_commit" => base_commit,
        "target_version" => target_version,
        "target_tag" => target_tag,
        "target_tag_object" => target_tag_object,
        "target_commit" => target_commit,
        "resolution_patch" => "postimages/semantic.patch",
        "resolution_patch_blob" => git(repository, "hash-object", postimage).strip,
        "generated_paths" => generated_contents.keys,
        "generated_postimages" => generated_contents.transform_values do |content|
          git_blob_oid(repository, content)
        end,
        "generation_profile" => generation_profile,
        "clean_preimages" => include_clean_preimage ? {
          "clean.txt" => git(repository, "rev-parse", ":clean.txt").strip
        } : {},
        "conflicts" => conflicts
      }
      if generation_profile == "ent_and_wire"
        manifest_data["ent_seed_paths"] = [
          "backend/ent/generated.go",
          "backend/ent/seed.go",
          *(auto_merged_generated ? ["backend/ent/group.go"] : [])
        ]
        manifest_data["ent_seed_preimages"] = manifest_data.fetch("ent_seed_paths").to_h do |path|
          [path, git(repository, "rev-parse", "HEAD:#{path}").strip]
        end
        if auto_merged_generated
          manifest_data["generated_preimages"] = {
            "backend/ent/group.go" => git(repository, "rev-parse", ":backend/ent/group.go").strip
          }
        end
      end
      File.write(manifest, JSON.pretty_generate(manifest_data) + "\n")

      fake_bin = File.join(dir, "bin")
      FileUtils.mkdir_p(fake_bin)
      File.write(File.join(fake_bin, "go"), <<~SH)
        #!/usr/bin/env bash
        set -euo pipefail
        [[ "$1" == "-C" ]]
        backend=$2
        shift 2
        case "$*" in
          "mod tidy") printf 'generated sum\n' > "$backend/go.sum" ;;
          *"entgo.io/ent/cmd/ent generate"*)
            printf 'generated ent\n' > "$backend/ent/generated.go"
            if [[ -f "$backend/ent/group.go" ]]; then
              printf 'generated group\n' > "$backend/ent/group.go"
            fi
            ;;
          "generate ./cmd/server") printf 'generated wire\n' > "$backend/cmd/server/wire_gen.go" ;;
          *) exit 64 ;;
        esac
      SH
      FileUtils.chmod(0o755, File.join(fake_bin, "go"))

      yield(
        repository: repository,
        records_root: records_root,
        manifest: manifest,
        fake_bin: fake_bin,
        base_commit: base_commit,
        target_commit: target_commit,
        target_tag_object: target_tag_object,
        path: "#{fake_bin}:#{ENV.fetch("PATH")}"
      )
    end
  end

  def resolver_fixture_files(repository)
    [
      "semantic.txt",
      "clean.txt",
      "backend/go.sum",
      "backend/cmd/server/wire_gen.go"
    ].select { |path| File.exist?(File.join(repository, path)) }
  end

  def unmerged_stages(repository)
    git(repository, "ls-files", "-u").lines.each_with_object({}) do |line, result|
      metadata, file = line.strip.split("\t", 2)
      _mode, blob, stage = metadata.split(" ")
      result[file] ||= {}
      result.fetch(file)[stage] = blob
    end
  end

  def run_resolver(fixture, target_version: "0.1.171", target_tag: "v0.1.171", target_commit: nil, target_tag_object: nil, base_commit: nil)
    target_commit ||= fixture.fetch(:target_commit)
    target_tag_object ||= fixture.fetch(:target_tag_object)
    base_commit ||= fixture.fetch(:base_commit)
    Open3.capture3(
      { "PATH" => fixture.fetch(:path) },
      "bash", RESOLVER,
      "--repository", fixture.fetch(:repository),
      "--records-root", fixture.fetch(:records_root),
      "--base-version", "0.1.169",
      "--base-commit", base_commit,
      "--target-version", target_version,
      "--target-tag", target_tag,
      "--target-tag-object", target_tag_object,
      "--target-commit", target_commit
    ).then { |stdout, stderr, status| [status, stdout + stderr] }
  end

  def with_repositories(
    conflict: false,
    generated_conflict: false,
    ignored_official_addition: false,
    base_version: "0.1.166",
    release_version: "0.1.167",
    release_tag: "v0.1.167",
    source_version: nil,
    source_version_symlink: false,
    same_official_commit: false
  )
    Dir.mktmpdir("merge-sub2api-release") do |dir|
      official = File.join(dir, "official")
      root = File.join(dir, "root")
      candidate_source_version = source_version || base_version
      FileUtils.mkdir_p([official, root])
      git(official, "init", "-q")
      configure_git(official)
      File.write(File.join(official, "app.txt"), "base\n")
      File.write(File.join(official, "common.txt"), "base\n")
      version_path = File.join(official, "backend/cmd/server/VERSION")
      FileUtils.mkdir_p(File.dirname(version_path))
      File.write(version_path, "#{candidate_source_version}\n")
      File.write(File.join(official, ".gitignore"), "docs/*\n") if ignored_official_addition
      if generated_conflict
        server = File.join(official, "backend/cmd/server")
        FileUtils.mkdir_p(server)
        File.write(File.join(official, "backend/go.mod"), "module example.invalid/fixture\n\ngo 1.24\n")
        File.write(File.join(server, "main.go"), <<~GO)
          package main

          //go:generate sh generate-wire.sh
        GO
        File.write(File.join(server, "generate-wire.sh"), <<~SH)
          printf '// Code generated by fixture. DO NOT EDIT.\npackage main\n\nconst generatedValue = "merged"\n' > wire_gen.go
        SH
        File.write(File.join(server, "wire_gen.go"), <<~GO)
          package main

          const generatedValue = "base"
        GO
      end
      git(official, "add", ".")
      git(official, "commit", "-q", "-m", "base")
      base_commit = git(official, "rev-parse", "HEAD").strip
      unless same_official_commit
        File.write(File.join(official, "app.txt"), "official\n")
        File.write(File.join(official, "new.txt"), "new\n")
        if ignored_official_addition
          FileUtils.mkdir_p(File.join(official, "docs"))
          File.write(File.join(official, "docs/official-safe-defaults.md"), "official tracked\n")
          git(official, "add", "-f", "docs/official-safe-defaults.md")
        end
        if generated_conflict
          File.write(File.join(official, "backend/cmd/server/wire_gen.go"), <<~GO)
            package main

            const generatedValue = "official"
          GO
        end
        git(official, "add", ".")
        git(official, "commit", "-q", "-m", "target")
      end
      target_commit = git(official, "rev-parse", "HEAD").strip
      git(official, "tag", "-a", release_tag, "-m", "release", target_commit)

      git(root, "init", "-q")
      configure_git(root)
      upstream = File.join(root, "upstream/sub2api")
      FileUtils.mkdir_p(upstream)
      File.write(File.join(upstream, "app.txt"), conflict ? "custom-conflict\n" : "base\n")
      File.write(File.join(upstream, "common.txt"), "base\n")
      File.write(File.join(upstream, "custom.txt"), "custom\n")
      version_path = File.join(upstream, "backend/cmd/server/VERSION")
      FileUtils.mkdir_p(File.dirname(version_path))
      File.write(version_path, "#{candidate_source_version}\n")
      if source_version_symlink
        stale_version_path = File.join(File.dirname(version_path), "STALE_VERSION")
        File.write(stale_version_path, "#{candidate_source_version}\n")
        FileUtils.rm_f(version_path)
        File.symlink("STALE_VERSION", version_path)
      end
      if ignored_official_addition
        File.write(File.join(upstream, ".gitignore"), "docs/*\n")
        FileUtils.mkdir_p(File.join(upstream, "docs"))
        File.write(File.join(upstream, "docs/local-ignored.md"), "local ignored\n")
      end
      if generated_conflict
        FileUtils.cp_r(
          File.join(official, "backend"),
          upstream
        )
        File.write(File.join(upstream, "backend/cmd/server/wire_gen.go"), <<~GO)
          package main

          const generatedValue = "custom"
        GO
      end
      File.write(File.join(upstream, "XINGQIAO_UPSTREAM.md"), <<~MD)
        # Xingqiao Upstream Source
        - Repository: `https://github.com/Wei-Shaw/sub2api.git`
        - Release tag: `v0.1.166`
        - Source commit: `#{base_commit}`
        - Annotated tag object: `#{"1" * 40}`
        - Imported: `2026-07-27`
      MD
      git(root, "add", ".")
      git(root, "commit", "-q", "-m", "root base")
      root_base = git(root, "rev-parse", "HEAD").strip

      metadata = File.join(dir, "metadata.json")
      File.write(metadata, JSON.generate(
        "has_update" => true,
        "base_sha" => root_base,
        "base_version" => base_version,
        "base_commit" => base_commit,
        "version" => release_version,
        "tag" => release_tag,
        "official_commit" => target_commit,
        "published_at" => "2026-07-28T01:02:03Z"
      ))
      yield(
        dir: dir, official: official, official_base: base_commit, root: root, root_base: root_base,
        metadata: metadata, bundle: File.join(dir, "candidate.bundle"),
        report: File.join(dir, "report.json")
      )
    end
  end

  def run_merge(fixture)
    Open3.capture3(
      "bash", MERGER,
      "--root", fixture[:root],
      "--metadata", fixture[:metadata],
      "--official-repository", fixture[:official],
      "--bundle", fixture[:bundle],
      "--report", fixture[:report]
    ).then { |stdout, stderr, status| [status, stdout + stderr] }
  end

  def configure_git(path)
    git(path, "config", "user.name", "Test")
    git(path, "config", "user.email", "test@example.invalid")
  end

  def git_blob_oid(path, content)
    stdout, stderr, status = Open3.capture3(
      "git", "-C", path, "hash-object", "--stdin", stdin_data: content
    )
    raise "git hash-object --stdin: #{stderr}" unless status.success?

    stdout.strip
  end

  def git(path, *arguments)
    stdout, stderr, status = Open3.capture3("git", "-C", path, *arguments)
    raise "git #{arguments.join(" ")}: #{stderr}" unless status.success?

    stdout
  end
end
