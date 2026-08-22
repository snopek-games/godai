package mcp

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/internal/jsonrpc"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

func fileURI(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

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

func TestClientRoots(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	staticRoot := filepath.Join(xdgBase, "static-root")
	decoy := filepath.Join(staticRoot, "decoy")
	mustCreateProject(t, decoy, "Decoy")

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

	instances := isolation.GodaiInstancesDir(xdgBase)
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

	instances := isolation.GodaiInstancesDir(xdgBase)
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

func TestClientRootsListChanged(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

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

	instances := isolation.GodaiInstancesDir(xdgBase)
	inst, err := startServerWithClient(xdgBase, []string{
		"--root", staticRoot,
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "", cfg)
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	eventually(t, func() bool {
		_, ok := listProjects(t, inst.client)[projectA]
		return ok
	})

	useB.Store(true)
	is.NoErr(inst.client.Notify(context.Background(), "notifications/roots/list_changed", map[string]any{}))

	eventually(t, func() bool {
		projects := listProjects(t, inst.client)
		_, hasA := projects[projectA]
		_, hasB := projects[projectB]
		return hasB && !hasA
	})
}

func TestListProjectsWithoutBasePath(t *testing.T) {
	is := is.New(t)

	xdgBase := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(xdgBase); err == nil {
		xdgBase = resolved
	}

	baseDir := filepath.Join(xdgBase, "base-projects")
	project := filepath.Join(baseDir, "from_base_path")
	mustCreateProject(t, project, "From Base Path")

	pmProject := filepath.Join(xdgBase, "elsewhere", "from_project_manager")
	mustCreateProject(t, pmProject, "From Project Manager")
	writeProjectsCfg(t, filepath.Join(isolation.GodotEditorDataDir(xdgBase), "projects.cfg"), pmProject)

	var elicitCalls atomic.Int32
	cfg := harness.ClientConfig{
		Capabilities: map[string]any{"elicitation": map[string]any{}},
		Handlers: map[string]harness.RequestHandler{
			"elicitation/create": func(params json.RawMessage) (any, *jsonrpc.Error) {
				elicitCalls.Add(1)
				return map[string]any{
					"action":  "accept",
					"content": map[string]any{"project_path": baseDir},
				}, nil
			},
		},
	}

	instances := isolation.GodaiInstancesDir(xdgBase)
	inst, err := startServerWithClient(xdgBase, []string{
		"--global",
		"--godot-path", godotWrapperPath,
		"--editor-instances-path", instances,
		"--editor-scan-interval", "1",
	}, nil, os.Getenv("GODAI_TEST_VERBOSE") != "", cfg)
	is.NoErr(err)
	t.Cleanup(func() { stopServer(inst.cmd) })

	structured := callToolOKWith(t, inst.client, "list_projects", nil)
	note, _ := structured["note"].(string)
	is.True(strings.Contains(note, "project manager"))   // the note explains the limited listing
	is.True(strings.Contains(note, "project_base_path")) // and how to fix it
	is.Equal(elicitCalls.Load(), int32(0))               // no elicitation, even though the client supports it
	is.Equal(listProjects(t, inst.client)[pmProject], "From Project Manager")

	out := callToolOKWith(t, inst.client, "set_godai_settings", map[string]any{
		"project_base_path": baseDir,
	})
	is.Equal(out["success"], true)

	structured = callToolOKWith(t, inst.client, "list_projects", nil)
	_, hasNote := structured["note"]
	is.True(!hasNote) // the note is gone once a base path is configured
	is.Equal(listProjects(t, inst.client)[project], "From Base Path")
}
