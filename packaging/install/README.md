Install scripts for godai
=========================

This directory contains the one-liner install scripts for the Go CLI, so
that users can install it without any package manager.

Linux and macOS:

```sh
curl -fsSL https://godai.sh/install | bash
```

Windows (PowerShell):

```powershell
irm https://godai.sh/install.ps1 | iex
```

The repository's raw URLs also work, as a fallback that needs no publish
step at all (pushing to `main` is enough):

```sh
curl -fsSL https://gitlab.com/snopek-games/godai/-/raw/main/packaging/install/install.sh | bash
irm https://gitlab.com/snopek-games/godai/-/raw/main/packaging/install/install.ps1 | iex
```

How the scripts work
--------------------

The scripts work for any release - they look up the latest one at run
time, so publishing is only needed when the scripts themselves change.

Both scripts follow the same flow, and the same conventions as the
installers for rustup, Deno, Bun, etc:

1. Detect the architecture (and on the sh side, the OS).
2. Resolve the version: the `GODAI_VERSION` environment variable if set,
   otherwise the latest release via the GitLab API's
   `releases/permalink/latest` endpoint.
3. Download that release's zip from the GitLab package registry (the same
   zips brew installs) plus its `checksums-vX.Y.Z.txt`, and verify the
   zip's sha256 - refusing to install on a mismatch.
4. Extract the binary to a per-user directory - no root/admin needed:
   `~/.local/bin/godai` on Linux/macOS,
   `%LOCALAPPDATA%\Programs\godai\godai.exe` on Windows.
5. Put the install dir on `PATH` if it isn't already: by appending an
   `export` line to `~/.zshrc`/`~/.bashrc`/`~/.bash_profile` (or printing
   instructions for other shells), or on Windows by updating the user
   `Path` environment variable. `GODAI_NO_MODIFY_PATH=1` disables this.

`GODAI_INSTALL_DIR` overrides the install directory, and
`GODAI_DOWNLOAD_BASE` overrides where the zips are downloaded from (which
is how CI tests the scripts without a real release). Environment variables
rather than flags, because neither `curl | sh` nor `irm | iex` can pass
arguments:

```sh
curl -fsSL https://gitlab.com/snopek-games/godai/-/raw/main/packaging/install/install.sh | GODAI_VERSION=1.2.3 sh
```

Updating after installation is `godai self-update` (or re-running the
script, which overwrites in place).

How the scripts reach godai.sh
------------------------------

The website is an Eleventy static site on GitLab Pages
(<https://gitlab.com/snopek-games/godai-website>). Its `src/install-scripts/`
directory is passthrough-copied to the site root at build time, which is
what serves `/install` and `/install.ps1`.

`website-publish` (release stage in `.gitlab-ci.yml`, runs on pushes to
`main` that touch `packaging/install/`) clones the website repo, copies
`install.sh` in as `install` and `install.ps1` as-is, and commits and
pushes if they changed. That push triggers the website's own pipeline,
which rebuilds and deploys Pages - so the live URLs update a few minutes
after a merge here.

GitLab Pages picks Content-Type by file extension and falls back to
content sniffing for extensions it doesn't know - which covers both the
extensionless `install` and `.ps1`, so both serve as `text/plain`. That
matters for `irm | iex`: PowerShell only returns text (rather than bytes)
for text content types. After changing how the scripts are served, verify
with:

```sh
curl -sI https://godai.sh/install | grep -i content-type
```

One-time setup
--------------

CI pushes to the website repo over SSH with a deploy key:

1. Generate a key pair (no passphrase; the private key never touches disk
   outside of CI):

   ```sh
   ssh-keygen -t ed25519 -N "" -C "godai website-publish" -f godai-website-deploy
   ```

2. In the godai-website project, add the *public* key
   (`godai-website-deploy.pub`) as a deploy key
   (Settings -> Repository -> Deploy keys), with
   **Grant write permissions to this key** checked.
3. Still in godai-website, allow the deploy key to push to the protected
   `main` branch: Settings -> Repository -> Protected branches -> `main` ->
   add the deploy key under **Allowed to push and merge**.
4. In *this* GitLab project, add the *private* key
   (`godai-website-deploy`) as a CI/CD variable
   (Settings -> CI/CD -> Variables): key `WEBSITE_DEPLOY_KEY`, type
   **File**, visibility **Visible** (private keys can't be masked - the
   File type keeps the content out of job logs anyway). Paste the whole
   file *including* the trailing newline: OpenSSH refuses keys without
   one, and it's easy to lose when copy-pasting.
5. Delete both local key files.

The job pins gitlab.com's ed25519 host key, so if GitLab ever rotates its
SSH host keys (last done in 2023 for RSA/ECDSA), the hardcoded key in
`.gitlab-ci.yml` needs updating to match
<https://docs.gitlab.com/user/gitlab_com/#ssh-host-keys-fingerprints>.

How CI tests the scripts
------------------------

`package-install` (package stage, every pipeline, in `.gitlab-ci.yml`)
runs shellcheck on `install.sh`, then does a real end-to-end install of
both scripts: it zips the freshly-built binaries into the same layout the
`release` job uploads, serves them from a local HTTP server, and runs each
script against it with `GODAI_DOWNLOAD_BASE` pointing at that server
(`install.ps1` runs under Linux PowerShell, with `PROCESSOR_ARCHITECTURE`
faked, so everything except the Windows PATH edit is exercised).

There is nothing to set up and no secrets: the scripts only ever *read*
public URLs.

Adding a new platform
---------------------

1. Add the build variant to the `cli-build` matrix in `.gitlab-ci.yml`.
2. Add it to the OS/arch detection `case` in `install.sh` (or the `$Arch`
   switch in `install.ps1`), mapping to the zip name's `<platform>-<arch>`.
