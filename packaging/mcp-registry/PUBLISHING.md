Publishing godai to the MCP registry
====================================

This directory contains the entry for the official MCP registry
(https://registry.modelcontextprotocol.io), which lets MCP clients discover
godai and install it via the npm package.

How it works
------------

- `server.json` declares the server under the `com.snopekgames/godai` name.
  The `version` fields are `0.0.0-dev` placeholders; CI stamps the real
  version in at publish time (same convention as the npm and mcpb packaging).
- It points at the `@snopek-games/godai` npm package, so the registry entry
  rides on the normal npm release - there is nothing extra to build.
- Two ownership proofs are involved:
  1. **Namespace**: `com.snopekgames/*` requires proving control of
     `snopekgames.com`, via an Ed25519 keypair whose public key is in a DNS
     TXT record on the domain apex (see setup below). The private key is what
     authenticates every publish.
  2. **Package**: the registry fetches `@snopek-games/godai` from npmjs.org
     and checks that its `package.json` contains
     `"mcpName": "com.snopekgames/godai"`. That field lives in
     `packaging/npm/package.json`. This also means the npm publish must
     complete *before* the registry publish for any given version.
- Registry versions are immutable: each publish must carry a version string
  that hasn't been published before.

In `.gitlab-ci.yml`:

- `package-mcp-registry` (package stage, every pipeline): stamps a dev
  version into `server.json` and runs `mcp-publisher validate`, so schema
  breakage is caught before release time.
- `mcp-registry-publish` (release stage, only on `vX.Y.Z` tags, after
  `npm-publish`): stamps the version from the git tag, logs in with the
  private key from the `MCP_REGISTRY_PRIVATE_KEY` CI variable, and runs
  `mcp-publisher publish`. The publish is retried a few times because the
  registry validates against npmjs.org, which may not have propagated the
  npm package published moments earlier.

One-time setup
--------------

### 1. Generate the keypair

```sh
openssl genpkey -algorithm Ed25519 -out mcp-registry-key.pem

# The public key, for the DNS TXT record:
openssl pkey -in mcp-registry-key.pem -pubout -outform DER | tail -c 32 | base64

# The private key as 64 hex characters, for the GitLab CI variable:
openssl pkey -in mcp-registry-key.pem -noout -text | grep -A3 "priv:" | tail -n +2 | tr -d ' :\n'; echo
```

Keep `mcp-registry-key.pem` (or the hex string, which is equivalent)
somewhere safe like a password manager - it is the only credential for the
`com.snopekgames/*` registry namespace. Don't commit it.

### 2. Add the DNS TXT record

On the **apex** of `snopekgames.com` (host/name `@` or blank in most DNS
control panels - NOT a subdomain like `_mcp-auth.snopekgames.com`; the
registry only looks at the apex), add a TXT record with the value:

```
v=MCPv1; k=ed25519; p=<public key from step 1>
```

This coexists fine with other TXT records on the apex (SPF, site
verifications, etc). Wait for propagation (usually minutes) and check with:

```sh
dig +short TXT snopekgames.com | grep MCPv1
```

If the key is ever rotated, delete the old TXT record - a stale record is
tried first and makes authentication fail.

### 3. Add the GitLab CI variable

In the GitLab project: **Settings -> CI/CD -> Variables -> Add variable**:

- **Key**: `MCP_REGISTRY_PRIVATE_KEY`
- **Value**: the 64-character hex private key from step 1
- **Type**: Variable (not File)
- **Flags**: Masked (the hex string satisfies the masking rules), and
  Protected if the `v*` tags are protected in this project

First publish
-------------

The registry's npm ownership check requires the *published* npm package to
contain the `mcpName` field, so the first registry publish can only happen
once a release containing that field is on npmjs.org. After that release's
`npm-publish` job succeeds, either let the `mcp-registry-publish` job do the
rest, or publish manually to see any validation errors interactively:

```sh
# Install mcp-publisher: `brew install mcp-publisher`, or download a binary
# from https://github.com/modelcontextprotocol/registry/releases

# From the repository root, with VERSION set to the release (no leading "v"):
jq --arg v "$VERSION" '.version = $v | .packages[].version = $v' \
  packaging/mcp-registry/server.json > server.json

mcp-publisher validate server.json
mcp-publisher login dns --domain snopekgames.com --private-key "<hex private key>"
mcp-publisher publish server.json
rm server.json
```

Verify the result with:

```sh
curl -s "https://registry.modelcontextprotocol.io/v0/servers?search=com.snopekgames/godai"
```

The same steps work as a fallback any time CI is broken - there is no
CI-only credential, unlike npm's trusted publishing.
