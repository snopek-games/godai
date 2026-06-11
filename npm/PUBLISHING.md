Publishing godai-mcp to npm
===========================

This directory contains everything related to distributing the Go MCP proxy
via npm, so that users can run it with `npx -y @snopek-games/godai-mcp`.

How the npm side works
----------------------

npm doesn't have first-class support for platform-specific binaries, so the
ecosystem convention (used by esbuild, Biome, Turborepo, etc) is:

- **One main package** (`@snopek-games/godai-mcp`) containing only a tiny
  Node.js launcher script. Its `bin` field maps the `godai-mcp` command to
  that script, which is what makes `npx @snopek-games/godai-mcp` work (when
  a package has a single bin, npx runs it regardless of its name).
- **One package per platform** (`@snopek-games/godai-mcp-linux-x64`,
  `@snopek-games/godai-mcp-darwin-arm64`, etc) containing just the compiled
  binary. Each declares `os` and `cpu` fields in its package.json, which
  tell npm "only install me on this platform".
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
  exist.
- `godai-mcp.js` - the launcher script.
- `README.md` - the README shown on npmjs.com for the main package.
- `prepare-packages.mjs` - generates the actual publishable packages into
  `npm/dist/` (gitignored): it stamps the version, creates the platform
  packages, and copies in the binaries from `dist/mcp/` (the artifacts of the
  `mcp-build` CI job).

How CI publishes a release
--------------------------

In `.gitlab-ci.yml`:

- `mcp-package-npm` (package stage, every pipeline): runs
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

1. Add the build variant to the `mcp-build` matrix in `.gitlab-ci.yml`.
2. Add `@snopek-games/godai-mcp-<process.platform>-<process.arch>` to
   `optionalDependencies` in `npm/package.json` (use Node's names: `darwin`
   not `macos`, `win32` not `windows`, `x64` not `x86_64`).
3. Map that package name to the CI variant in `VARIANTS` in
   `prepare-packages.mjs`.
4. Publish the new platform package manually once - npm won't let you
   configure a trusted publisher on a package that doesn't exist yet.
   Follow "Publishing manually" below, but only run `npm publish` in the
   *new* package's directory under `npm/dist/platforms/`. Use a version
   like `X.Y.Z-bootstrap` (where `X.Y.Z` is the upcoming release) so it
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
# Build the binaries for all platforms, the same way CI does:
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/mcp/godai-mcp-linux-x86_64/godai-mcp-linux-x86_64 ./mcp/
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/mcp/godai-mcp-linux-arm64/godai-mcp-linux-arm64 ./mcp/
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/mcp/godai-mcp-windows-x86_64/godai-mcp-windows-x86_64.exe ./mcp/
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/mcp/godai-mcp-windows-arm64/godai-mcp-windows-arm64.exe ./mcp/
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/mcp/godai-mcp-macos-arm64/godai-mcp-macos-arm64 ./mcp/

# Generate the npm packages with the right version stamped in
# (npm never allows re-publishing a version, even a deleted one):
node npm/prepare-packages.mjs <version>

# Log in to npmjs.com (opens a browser; only needed once per machine):
npm login

# Publish the platform packages first, then the main package:
for dir in npm/dist/platforms/*/; do
  (cd "$dir" && npm publish --access public)
done
(cd npm/dist/godai-mcp && npm publish --access public)
```

With 2FA enabled, npm asks you to confirm each of the 6 publishes in the
browser (or pass `--otp <code>` from your authenticator app to each
`npm publish`).

To just test the packaging without publishing, run the build and
prepare-packages steps with a version like `0.0.0-test`, then
`npm pack` (instead of `npm publish`) in any of the `npm/dist` package
directories produces a .tgz you can inspect or `npm install` directly.
