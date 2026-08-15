Publishing godai to Homebrew
============================

This directory contains everything related to distributing the Go CLI via
[Homebrew](https://brew.sh/), so that users can install it with:

```sh
brew tap snopek-games/godai
brew install godai
```

and update it with `brew update && brew upgrade godai`.

How the Homebrew side works
---------------------------

Homebrew has two package types: **formulae** (the classic kind; work on both
macOS and Linux) and **casks** (prebuilt macOS-only artifacts, mostly GUI
apps). We ship a formula, for two reasons:

- Formulae work with Homebrew on Linux; casks don't.
- Cask downloads get macOS's quarantine attribute, so Gatekeeper blocks our
  unsigned binaries ("cannot verify the developer") unless they're signed
  with an Apple Developer ID and notarized. Formula downloads aren't
  quarantined, so the plain Go binaries just work.

A formula normally builds from source, but ours simply downloads the prebuilt
release zip for the user's OS/arch (the same zips the `release` CI job uploads
to the GitLab package registry) and installs the `godai` binary from it. The
formula pins the sha256 of every zip, and brew verifies the download against
it.

Getting a package into Homebrew's *official* repositories means a GitHub pull
request, notability requirements, and their review process for every update.
The standard alternative for a project like this is a **tap**: a git repo of
your own that users add to their brew with `brew tap`. Ours is
<https://github.com/snopek-games/homebrew-godai> - the `homebrew-` prefix is what
lets users write just `brew tap snopek-games/godai`. All a tap needs is a
`Formula/` directory containing formula files; CI keeps
`Formula/godai.rb` there up to date.

Files in this directory
-----------------------

- `godai.rb.tmpl` - the formula, with `@VERSION@` and `@SHA256_*@`
  placeholders.
- `prepare-formula.sh` - renders the template: takes a version, a
  `sha256sum`-format checksums file, and an output path, and fails if any
  platform's zip is missing from the checksums.

How CI publishes a release
--------------------------

In `.gitlab-ci.yml`:

- `package-brew` (package stage, every pipeline): zips the freshly-built
  binaries, renders the formula from their checksums with a dev version, and
  runs `ruby -c` on the result, so template/script breakage is caught before
  release time.
- `brew-publish` (release stage, only on `vX.Y.Z` tags): renders the formula
  using the version from the git tag and the `checksums-<tag>.txt` the
  `release` job generated (so the sha256s are exactly those of the uploaded
  zips), then clones the tap repo, commits the new `Formula/godai.rb`, and
  pushes.

One-time setup
--------------

1. Create the tap repo: <https://github.com/snopek-games/homebrew-godai>, public,
   initialized with a README (so the default branch exists before CI's first
   push).
2. Create a GitHub fine-grained personal access token for CI to push with:
   GitHub -> Settings -> Developer settings -> Personal access tokens ->
   Fine-grained tokens. Set the resource owner to the `snopek-games`
   organization (the org has to allow fine-grained tokens for this to be
   offered), limit it to only the `homebrew-godai` repository, and grant it
   just **Contents: Read and write**.
3. In the GitLab project, add the token as a CI/CD variable
   (Settings -> CI/CD -> Variables): key `BREW_TAP_GITHUB_TOKEN`, **Masked**.
   Only mark it **Protected** if the `v*` tags are protected too - a
   protected variable is invisible to pipelines for unprotected tags, and
   the publish would fail.

Fine-grained tokens expire (max one year), so this token needs rotating:
when `brew-publish` starts failing with authentication errors, generate a
new token and update the variable.

Adding a new platform
---------------------

1. Add the build variant to the `cli-build` matrix in `.gitlab-ci.yml`.
2. Add an `on_*` block for it in `godai.rb.tmpl`, with a new placeholder.
3. Fill that placeholder in `prepare-formula.sh` from the matching zip name.

Publishing manually
-------------------

A fallback if CI is broken. The release must already exist (the formula
points at its uploaded zips). Run from the repository root:

```sh
TAG=vX.Y.Z
curl -fLO "https://gitlab.com/api/v4/projects/snopek-games%2Fgodai/packages/generic/release-packages/${TAG}/checksums-${TAG}.txt"
packaging/brew/prepare-formula.sh "${TAG#v}" "checksums-${TAG}.txt" godai.rb

git clone git@github.com:snopek-games/homebrew-godai.git /tmp/homebrew-godai
cp godai.rb /tmp/homebrew-godai/Formula/godai.rb
cd /tmp/homebrew-godai
git add Formula/godai.rb
git commit -m "godai ${TAG}"
git push
```

To test a rendered formula without publishing, point brew straight at the
file on a machine with brew installed:

```sh
brew install --formula ./godai.rb
brew test godai
```
