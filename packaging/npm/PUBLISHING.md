Publishing godai to npm
===========================

This directory contains everything related to distributing the Go CLI via
npm, so that users can run it with `npx -y @snopek-games/godai`.

How the npm side works
----------------------

npm doesn't have first-class support for platform-specific binaries, so the
ecosystem convention (used by esbuild, Biome, Turborepo, etc) is:

- **One main package** (`@snopek-games/godai`) containing only a tiny
  Node.js launcher script. Its `bin` field maps the `godai` command to
  that script, which is what makes `npx @snopek-games/godai` work (when
  a package has a single bin, npx runs it regardless of its name).
- **One package per platform** (`@snopek-games/godai-linux-x64`,
  `@snopek-games/godai-darwin-arm64`, etc) containing the compiled binary
  and a `godai-install-channel` file, which makes `godai self-update` defer
  to npm instead of replacing the executable behind npm's back. Each declares
  `os` and `cpu` fields in its package.json, which tell npm "only install me
  on this platform".
- The main package lists all the platform packages as `optionalDependencies`.
  When a user installs the main package, npm tries to install all of them but
  silently skips the ones whose `os`/`cpu` don't match - so each user only
  downloads the one binary for their system.
- At runtime the launcher builds the package name from `process.platform` and
  `process.arch`, resolves the installed platform package, and spawns the
  binary with stdio passed through.

All packages are published with the **same version number** on every release,
and the main package pins its optionalDependencies to that exact version.

Files in this directory
-----------------------

- `package.json` - the main package. The `version` is a `0.0.0-dev`
  placeholder; CI stamps the real version in at publish time. Its
  `optionalDependencies` keys are the source of truth for which platforms
  exist. Its `mcpName` field is how the MCP registry verifies we own this
  package (see `packaging/mcp-registry/PUBLISHING.md`).
- `godai.js` - the launcher script.
- `README.md` - the README shown on npmjs.com for the main package.
- `prepare-packages.mjs` - generates the actual publishable packages into
  `packaging/npm/dist/` (gitignored): it stamps the version, creates the platform
  packages, and copies in the binaries from `dist/cli/` (the artifacts of the
  `cli-build` CI job).

How CI publishes a release
--------------------------

In `.gitlab-ci.yml`:

- `package-npm` (package stage, every pipeline): runs
  `prepare-packages.mjs` with a dev version and does `npm pack --dry-run` on
  every package, so breakage is caught before release time.
- `npm-publish` (release stage, only on `vX.Y.Z` tags): re-runs
  `prepare-packages.mjs` with the version taken from the git tag (minus the
  leading `v`), then runs `npm publish` for each platform package and finally
  the main package. The platform packages are published first so that the
  main package's optionalDependencies always resolve. Authentication uses
  trusted publishing - see the next section.

How CI authenticates: trusted publishing
----------------------------------------

CI publishes via npm's "trusted publishing" (OIDC): instead of a stored
token, GitLab gives the `npm-publish` job a short-lived, signed identity
token (the `id_tokens:` block in the job), and npm accepts the publish
because the job's identity matches a "trusted publisher" configured on the
package. There is no secret to store in GitLab or rotate, and npm
automatically attaches provenance attestations to every publish.

For this to work, **each of the packages** (the main one and every platform
package) must have a trusted publisher configured on npmjs.com:

1. Go to the package's page (while logged in with publish rights) ->
   **Settings** -> **Trusted Publisher**.
2. Select **GitLab CI/CD** and fill in:
   - **Namespace**: `snopek-games`
   - **Project name**: `godai`
   - **Top-level CI file path**: `.gitlab-ci.yml`
   - **Environment name**: leave empty
3. Allow the **npm publish** action.

Caveats:

- A trusted publisher can only be configured on a package that **already
  exists** on npmjs.com, so the first publish of any *new* package must be
  done manually (see "Publishing manually" below).
- Trusted publishing only works from GitLab.com shared runners (not
  self-hosted runners), and needs npm >= 11.5.1 and Node >= 22.14 (the
  `node:current` image satisfies both).
- The npm CLI detects the OIDC environment automatically - `npm publish`
  needs no extra flags or `.npmrc`. (Don't configure a token in the job:
  if one is present, npm uses it instead of OIDC.)

With that in place, pushing a `vX.Y.Z` tag publishes everything
automatically.

Adding a new platform
---------------------

1. Add the build variant to the `cli-build` matrix in `.gitlab-ci.yml`.
2. Add `@snopek-games/godai-<process.platform>-<process.arch>` to
   `optionalDependencies` in `packaging/npm/package.json` (use Node's names: `darwin`
   not `macos`, `win32` not `windows`, `x64` not `x86_64`).
3. Map that package name to the CI variant in `VARIANTS` in
   `prepare-packages.mjs`.
4. Publish the new platform package manually once - npm won't let you
   configure a trusted publisher on a package that doesn't exist yet.
   Follow "Publishing manually" below, but only run `npm publish` in the
   *new* package's directory under `packaging/npm/dist/platforms/`. Use a version
   like `X.Y.Z-dev1` (where `X.Y.Z` is the upcoming release) so it
   can't collide with a real release; the version doesn't matter otherwise,
   since installs always use the exact versions pinned by the main
   package's optionalDependencies.
5. On npmjs.com, configure the trusted publisher for the new package (see
   "How CI authenticates" above).

After that, the new platform is published automatically as part of the next
release.

Publishing manually
-------------------

Used to bootstrap the very first release (a package must exist on npmjs.com
before a trusted publisher can be configured for it), but also works as a
fallback if CI is broken. Run from the repository root:

```sh
# Only when publishing a pre-release version: stamp it into plugin.cfg, which
# is where the binary and the addon it installs get their version from. Revert
# this afterwards - the version-check CI job requires plugin.cfg to match the
# release tag exactly.
sed -i 's/^version=".*"$/version="<version>"/' addons/godai/plugin.cfg

# Build the binaries for all platforms, the same way CI does (clearing the
# output first, so a stale binary can't get published):
rm -rf dist/cli
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -tags selfupdate -trimpath -ldflags="-s -w" -o dist/cli/godai-cli-linux-x86_64/godai ./cmd/godai/
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -tags selfupdate -trimpath -ldflags="-s -w" -o dist/cli/godai-cli-linux-arm64/godai ./cmd/godai/
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags selfupdate -trimpath -ldflags="-s -w" -o dist/cli/godai-cli-windows-x86_64/godai.exe ./cmd/godai/
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -tags selfupdate -trimpath -ldflags="-s -w" -o dist/cli/godai-cli-windows-arm64/godai.exe ./cmd/godai/
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -tags selfupdate -trimpath -ldflags="-s -w" -o dist/cli/godai-cli-macos-arm64/godai ./cmd/godai/
CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -tags selfupdate -trimpath -ldflags="-s -w" -o dist/cli/godai-cli-macos-x86_64/godai ./cmd/godai/

# Generate the npm packages with the right version stamped in
# (npm never allows re-publishing a version, even a deleted one):
node packaging/npm/prepare-packages.mjs <version>

# Log in to npmjs.com (opens a browser; only needed once per machine):
npm login

# Publish the platform packages first, then the main package:
for dir in packaging/npm/dist/platforms/*/; do
  (cd "$dir" && npm publish --access public)
done
(cd packaging/npm/dist/godai && npm publish --access public)

# Revert the plugin.cfg change, if you made one:
git checkout -- addons/godai/plugin.cfg
```

With 2FA enabled, npm asks you to confirm each of the 6 publishes in the
browser (or pass `--otp <code>` from your authenticator app to each
`npm publish`).

Letting a pre-release build claim the upcoming release's version instead of
its own breaks two things: `installAddon` replaces a project's addon only when
the embedded and installed versions differ as strings, so a project bootstrapped
from the pre-release would keep a stale addon; and the update check only reports
a release that sorts *above* the running version, so nobody would be told to
upgrade to the real release.

To just test the packaging without publishing, run the build and
prepare-packages steps with a version like `0.0.0-test`, then
`npm pack` (instead of `npm publish`) in any of the `packaging/npm/dist` package
directories produces a .tgz you can inspect or `npm install` directly.
