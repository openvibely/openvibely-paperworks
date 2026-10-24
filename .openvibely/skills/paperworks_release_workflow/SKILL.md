---
kind: openvibely.agent_skill
version: 1
skill:
  key: paperworks_release_workflow
  name: Paperworks Release Workflow
  scope: project
  description: Build, sign, notarize, verify, and publish a stable Paperworks release for macOS, Linux, and Windows on amd64 and arm64.
---

# Paperworks release

Use for stable Paperworks releases. The user supplies X.Y.Z. Keep binaries in ignored `dist/` and GitHub assets, never in source control. Do not print signing credentials or tokens.

1. Inspect git status, branch, recent commits, and remote main. Release the intended committed main revision. Check that the version tag and GitHub release do not already exist; never move an existing release tag implicitly.
2. Run `bash scripts/test-local.sh`. Fix failures before releasing. Run `bash scripts/release-build.sh X.Y.Z --dry-run` and review the six-target matrix.
3. Run `bash scripts/release-build.sh X.Y.Z`. The macOS coordinator uses the sibling OV release workflow's Developer ID, Apple notarization, Azure Authenticode, and signature-verification helpers. Override the sibling location with `PAPERWORKS_OV_ROOT`; override its private `.release-signing.env` with `PAPERWORKS_SIGNING_ENV`. Existing OV credentials are reused, never copied into this repository. If tooling is missing, run the OV signing setup with `SKIP_WAILS_DOCKER_SETUP=1`.
4. Inspect all six ZIPs and SHA256SUMS. Each ZIP contains `paperworks-OS-ARCH` (`.exe` on Windows). Verify extracted macOS binaries with `codesign --verify --strict`, and Windows executables with OV's `verify-windows.sh`. Both Apple reports must say `Accepted`. ZIPs containing standalone executables cannot be stapled; retain the Apple submission IDs in the local notarization reports. Linux artifacts have SHA-256 checksums, not Authenticode or Developer ID signatures.
5. Smoke-test the native signed executable (`version`, and an actual PDF or Office operation). Cross-compilation and signature verification do not prove native execution on Windows/Linux; report that limitation unless native tests were run.
6. Write release-specific `dist/X.Y.Z/RELEASE_NOTES.md` from the actual commit range. Describe installation/runtime dependencies accurately. Verify `BUILD_COMMIT` equals the intended tag commit, the worktree is clean, and `shasum -a 256 -c SHA256SUMS` passes from the release directory.
7. When release publishing is authorized, create an annotated `vX.Y.Z` tag at `BUILD_COMMIT`, push that tag to the verified Paperworks remote, and use `gh release create --verify-tag` with the notes, six ZIPs, six buildinfo files, BUILD_COMMIT, and SHA256SUMS. Use a stable release (not `--prerelease`). Do not upload private signing configuration. Confirm uploaded asset sizes/digests match local files and the release tag points to BUILD_COMMIT.

If notarization is interrupted, preserve `dist/X.Y.Z` and recover the submitted job with `xcrun notarytool info/wait/log` using its ID. Do not publish unaccepted archives or regenerate a previously submitted archive silently. Failures before publication leave local artifacts for inspection; do not publish a partial platform matrix.

Update OV dependency pins and checksums when requested, using the final signed archives. Signing changes bytes, so never reuse checksums from unsigned builds.
