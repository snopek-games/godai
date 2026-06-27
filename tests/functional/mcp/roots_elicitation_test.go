package mcp

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matryer/is"

	"godai/mcp/jsonrpc"
	"godai/tests/functional/internal/harness"
)

// fileURI builds a "file://" URI for an absolute path, the way an MCP client
// advertises a root.
func fileURI(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

// eventually polls cond until it returns true, failing the test if it hasn't
// within a few seconds. Used to wait out the asynchronous handling of a
// server->client notification.
func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

// TestClientRoots drives the server with a client that supports "roots" (the
// server->client request direction). The server asks the client for its roots
// via roots/list, and those replace whatever was passed on --root, so
// list_projects discovers projects under the client-provided root.
func TestClientRoots(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	// A project only reachable via the static --root, which the client's roots
	// should replace.
	staticRoot := filepath.Join(xdgBase, "static-root")
	decoy := filepath.Join(staticRoot, "decoy")
	mustCreateProject(t, decoy, "Decoy")

	// A project reachable only via the root the client reports.
	clientRoot := filepath.Join(xdgBase, "client-root")
	clientProject := filepath.Join(clientRoot, "from_client_roots")
	mustCreateProject(t, clientProject, "From Client Roots")

	var rootsListCalls atomic.Int32
	cfg := harness.ClientConfig{
		// No "listChanged", so the server re-fetches roots on demand (getRootPaths).
		Capabilities: map[string]any{"roots": map[string]any{}},
		Handlers: map[string]harness.RequestHandler{
			"roots/list": func(params json.RawMessage) (any, *jsonrpc.Error) {
				rootsListCalls.Add(1)
				return map[string]any{
					"roots": []map[string]any{
						{"uri": fileURI(clientRoot), "name": "Client Root"},
					},
				}, nil
			},
		},
	}

	instances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	inst, err := startServerWithClient(xdgBase, []string{
		"--root", staticRoot,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "", cfg)
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	projects := listProjects(t, inst.client)
	is.Equal(projects[clientProject], "From Client Roots") // found via client-provided roots
	_, decoyFound := projects[decoy]
	is.True(!decoyFound)               // the static --root was replaced
	is.True(rootsListCalls.Load() > 0) // the server actually called roots/list
}

// TestClientRootsUnsupported drives the server with a client that advertises the
// "roots" capability but doesn't actually answer roots/list (no handler, so the
// server gets MethodNotFound). The server marks roots unsupported and falls back
// to the static --root.
func TestClientRootsUnsupported(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	staticRoot := filepath.Join(xdgBase, "static-root")
	staticProject := filepath.Join(staticRoot, "from_static_root")
	mustCreateProject(t, staticProject, "From Static Root")

	cfg := harness.ClientConfig{
		// Advertise roots, but provide no handler: roots/list -> MethodNotFound.
		Capabilities: map[string]any{"roots": map[string]any{}},
	}

	instances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	inst, err := startServerWithClient(xdgBase, []string{
		"--root", staticRoot,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "", cfg)
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	projects := listProjects(t, inst.client)
	is.Equal(projects[staticProject], "From Static Root") // fell back to the static --root
}

// TestClientRootsListChanged exercises the roots list_changed path: a client
// that supports listChanged caches its roots on the server, and only a
// notifications/roots/list_changed prompts a refresh (rpcRootsListChanged).
func TestClientRootsListChanged(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	// Two candidate roots; the client switches which one it reports after it
	// sends the list_changed notification.
	rootA := filepath.Join(xdgBase, "root-a")
	projectA := filepath.Join(rootA, "project_a")
	mustCreateProject(t, projectA, "Project A")

	rootB := filepath.Join(xdgBase, "root-b")
	projectB := filepath.Join(rootB, "project_b")
	mustCreateProject(t, projectB, "Project B")

	// An empty static root, so nothing is found until the first roots/list.
	staticRoot := filepath.Join(xdgBase, "static-root")
	if err := os.MkdirAll(staticRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	var useB atomic.Bool
	cfg := harness.ClientConfig{
		// listChanged=true: the server caches roots and only re-fetches when we
		// notify it, so this is the only path that updates the roots.
		Capabilities: map[string]any{"roots": map[string]any{"listChanged": true}},
		Handlers: map[string]harness.RequestHandler{
			"roots/list": func(params json.RawMessage) (any, *jsonrpc.Error) {
				root := rootA
				if useB.Load() {
					root = rootB
				}
				return map[string]any{
					"roots": []map[string]any{{"uri": fileURI(root), "name": "Root"}},
				}, nil
			},
		},
	}

	instances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	inst, err := startServerWithClient(xdgBase, []string{
		"--root", staticRoot,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "", cfg)
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	// The server fetches roots once after initialize: Project A shows up.
	eventually(t, func() bool {
		_, ok := listProjects(t, inst.client)[projectA]
		return ok
	})

	// Switch the reported root and notify the server. With listChanged on, only
	// rpcRootsListChanged can make the cached roots change.
	useB.Store(true)
	is.NoErr(inst.client.Notify(context.Background(), "notifications/roots/list_changed", map[string]any{}))

	eventually(t, func() bool {
		projects := listProjects(t, inst.client)
		_, hasA := projects[projectA]
		_, hasB := projects[projectB]
		return hasB && !hasA
	})
}

// TestClientElicitation drives a --global server with a client that supports
// form elicitation but no configured project base path. list_projects triggers
// the server to elicit the base path from the client; the elicited path is then
// used (and saved) so the project under it is discovered.
func TestClientElicitation(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	baseDir := filepath.Join(xdgBase, "elicited-base")
	project := filepath.Join(baseDir, "from_elicitation")
	mustCreateProject(t, project, "From Elicitation")

	var elicitCalls atomic.Int32
	cfg := harness.ClientConfig{
		Capabilities: map[string]any{"elicitation": map[string]any{}},
		Handlers: map[string]harness.RequestHandler{
			"elicitation/create": func(params json.RawMessage) (any, *jsonrpc.Error) {
				elicitCalls.Add(1)
				// The server asks for project_path (see getProjectBasePath).
				return map[string]any{
					"action":  "accept",
					"content": map[string]any{"project_path": baseDir},
				}, nil
			},
		},
	}

	instances := filepath.Join(xdgBase, "cache", "godai-mcp", "instances")
	inst, err := startServerWithClient(xdgBase, []string{
		"--global",
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "", cfg)
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	// In global mode list_projects asks for the base path; the client supplies it.
	projects := listProjects(t, inst.client)
	is.Equal(projects[project], "From Elicitation") // found via the elicited base path
	is.True(elicitCalls.Load() > 0)                 // the server actually elicited

	// The elicited path was saved into the configuration.
	cfg2 := getConfig(t, inst.client)
	is.Equal(cfg2["project_base_path"], baseDir)
}
