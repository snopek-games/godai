package godot

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matryer/is"
)

// writeInstance writes an instance file (matching the format written by the
// Godot addon) into dir, advertising an editor listening on the given port.
// The PID is this (running) test process, so the instance is treated as live.
func writeInstance(t *testing.T, dir, instanceID string, port int) {
	t.Helper()
	writeInstanceWithPID(t, dir, instanceID, port, os.Getpid())
}

// writeInstanceWithPID is like writeInstance but records a specific PID, used to
// simulate an editor that's no longer running.
func writeInstanceWithPID(t *testing.T, dir, instanceID string, port, pid int) {
	t.Helper()
	b, err := json.Marshal(instance{
		InstanceID:  instanceID,
		PID:         pid,
		ProjectPath: "/tmp/project-" + instanceID,
		Secret:      "secret-" + instanceID,
		Port:        port,
	})
	if err != nil {
		t.Fatalf("marshalling instance: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, instanceID+".json"), b, 0o644); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}
}

// writeInstanceWithProject is like writeInstance but records a specific project
// path, used to exercise the ProjectConnectionScanner's root filtering.
func writeInstanceWithProject(t *testing.T, dir, instanceID string, port int, projectPath string) {
	t.Helper()
	b, err := json.Marshal(instance{
		InstanceID:  instanceID,
		PID:         os.Getpid(),
		ProjectPath: projectPath,
		Secret:      "secret-" + instanceID,
		Port:        port,
	})
	if err != nil {
		t.Fatalf("marshalling instance: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, instanceID+".json"), b, 0o644); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}
}

// deadPID returns a PID that is guaranteed not to be running, by starting a
// short-lived helper process and waiting for it to exit.
func deadPID(t *testing.T) int {
	t.Helper()
	// Re-run the test binary with a filter that matches no tests, so it exits
	// (almost) immediately.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting helper process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("waiting for helper process: %v", err)
	}
	return pid
}

func removeInstance(t *testing.T, dir, instanceID string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, instanceID+".json")); err != nil {
		t.Fatalf("removing instance file: %v", err)
	}
}

type MockEditor struct {
	addr     string
	upgrader websocket.Upgrader
	errCh    chan error
	server   *http.Server

	// mutex guards conn, which is written by the HTTP handler goroutine and
	// read/cleared by Stop() on the test goroutine.
	mutex sync.Mutex
	conn  *websocket.Conn
}

func NewMockEditor(addr string) *MockEditor {
	m := &MockEditor{
		addr: addr,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		errCh: make(chan error, 1),
	}
	return m
}

func (m *MockEditor) handleRequest(w http.ResponseWriter, r *http.Request) {
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("unable to upgrade to WebSocket", "error", err)
	}
	m.mutex.Lock()
	m.conn = conn
	m.mutex.Unlock()
	// @todo Do something with this connection
}

func (m *MockEditor) Start() {
	if m.server != nil {
		return
	}
	m.server = &http.Server{
		Addr:    m.addr,
		Handler: http.HandlerFunc(m.handleRequest),
	}
	go func() {
		slog.Info("starting editor", "addr", m.addr)
		err := m.server.ListenAndServe()
		m.errCh <- err
	}()
}

func (m *MockEditor) Stop() error {
	m.mutex.Lock()
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}
	m.mutex.Unlock()

	if m.server != nil {
		m.server.Close()
		m.server = nil

		err := <-m.errCh
		return err
	}

	return errors.New("cannot stop editor that hasn't been started")
}

func setupTestLogger() {
	// Show the debug log level. Won't appear without `-v` option to `go test`.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}

type connectionList struct {
	conns []*Connection
	mutex sync.RWMutex
}

func newConnectionList(initialCapacity int) *connectionList {
	return &connectionList{
		conns: make([]*Connection, 0, initialCapacity),
	}
}

func (l *connectionList) addConn(conn *Connection) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.conns = append(l.conns, conn)

	return nil
}

func (l *connectionList) removeConn(conn *Connection) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Filter out the removed connection.
	newConns := make([]*Connection, 0, len(l.conns)-1)
	for _, c := range l.conns {
		if c != conn {
			newConns = append(newConns, c)
		}
	}
	l.conns = newConns
}

func (l *connectionList) GetFirstConn() *Connection {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if len(l.conns) == 0 {
		return nil
	}
	return l.conns[0]
}

func (l *connectionList) GetConnections() []*Connection {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	connsCopy := make([]*Connection, len(l.conns))
	copy(connsCopy, l.conns)

	return connsCopy
}

func TestIsProcessRunning(t *testing.T) {
	is := is.New(t)

	is.True(isProcessRunning(os.Getpid())) // our own process is running
	is.True(!isProcessRunning(deadPID(t))) // a process that has exited
	is.True(!isProcessRunning(-1))         // an invalid PID
}

func TestConnectionManagerSingle(t *testing.T) {
	setupTestLogger()

	is := is.New(t)

	// Advertise an editor on port 13000 via an instance file.
	dir := t.TempDir()
	writeInstance(t, dir, "single", 13000)

	s := NewMockEditor("localhost:13000")
	l := newConnectionList(1)
	m := NewConnectionManager(ConnectionManagerConfig{
		Scanner:      &GlobalConnectionScanner{InstancesPath: dir},
		ScanInterval: 500 * time.Millisecond,
		RetryDelay:   1 * time.Second,
		OnConnect:    l.addConn,
		OnDisconnect: l.removeConn,
	})
	var conn *Connection

	m.Start()
	defer m.Stop()

	time.Sleep(2 * time.Second)

	// Shouldn't have any connections - the editor is stopped.
	conn = l.GetFirstConn()
	is.True(conn == nil)

	// Start editor - we should get a connection now!
	s.Start()
	defer s.Stop()
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn != nil)

	// Stop the editor - connection should be lost.
	err := s.Stop()
	slog.Debug("editor stopped", "error", err)
	is.True(errors.Is(err, http.ErrServerClosed))
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn == nil)

	// Start editor again - should reconnect!
	s.Start()
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn != nil)

	m.Stop()
}

// TestConnectionManagerInstanceRemoved verifies that removing an editor's
// instance file disconnects it, even while the editor is still running.
func TestConnectionManagerInstanceRemoved(t *testing.T) {
	setupTestLogger()

	is := is.New(t)

	dir := t.TempDir()
	writeInstance(t, dir, "single", 13005)

	s := NewMockEditor("localhost:13005")
	s.Start()
	defer s.Stop()

	l := newConnectionList(1)
	m := NewConnectionManager(ConnectionManagerConfig{
		Scanner:      &GlobalConnectionScanner{InstancesPath: dir},
		ScanInterval: 500 * time.Millisecond,
		RetryDelay:   1 * time.Second,
		OnConnect:    l.addConn,
		OnDisconnect: l.removeConn,
	})

	m.Start()
	defer m.Stop()

	// The editor is running and advertised, so we should connect.
	time.Sleep(2 * time.Second)
	is.True(l.GetFirstConn() != nil)

	// Remove the instance file - the connection should be dropped even though
	// the editor itself is still running.
	removeInstance(t, dir, "single")
	time.Sleep(2 * time.Second)
	is.True(l.GetFirstConn() == nil)
}

// TestConnectionManagerStaleInstanceRemoved verifies that an instance file whose
// editor process is no longer running is treated as stale: we don't connect to
// it (even if something is listening on its port), and the file is removed.
func TestConnectionManagerStaleInstanceRemoved(t *testing.T) {
	setupTestLogger()

	is := is.New(t)

	dir := t.TempDir()
	writeInstanceWithPID(t, dir, "stale", 13007, deadPID(t))

	// Something is listening on the advertised port, to show that it's the dead
	// PID - not the absence of a server - that keeps us from connecting.
	s := NewMockEditor("localhost:13007")
	s.Start()
	defer s.Stop()

	l := newConnectionList(1)
	m := NewConnectionManager(ConnectionManagerConfig{
		Scanner:      &GlobalConnectionScanner{InstancesPath: dir},
		ScanInterval: 500 * time.Millisecond,
		RetryDelay:   1 * time.Second,
		OnConnect:    l.addConn,
		OnDisconnect: l.removeConn,
	})

	m.Start()
	defer m.Stop()

	time.Sleep(2 * time.Second)

	// We never connect to a stale instance...
	is.True(l.GetFirstConn() == nil)

	// ...and the stale instance file has been cleaned up.
	_, err := os.Stat(filepath.Join(dir, "stale.json"))
	is.True(errors.Is(err, os.ErrNotExist))
}

func TestConnectionManagerMultiple(t *testing.T) {
	setupTestLogger()

	is := is.New(t)

	s1 := NewMockEditor("localhost:13010")
	s2 := NewMockEditor("localhost:13013")

	// Advertise both editors up-front; only the ones actually running will
	// produce a connection.
	dir := t.TempDir()
	writeInstance(t, dir, "editor-1", 13010)
	writeInstance(t, dir, "editor-2", 13013)

	l := newConnectionList(2)
	m := NewConnectionManager(ConnectionManagerConfig{
		Scanner:      &GlobalConnectionScanner{InstancesPath: dir},
		ScanInterval: 500 * time.Millisecond,
		RetryDelay:   1 * time.Second,
		OnConnect:    l.addConn,
		OnDisconnect: l.removeConn,
	})
	var conn *Connection
	var listc []*Connection

	m.Start()
	defer m.Stop()

	time.Sleep(2 * time.Second)

	// Shouldn't have any connections.
	conn = l.GetFirstConn()
	is.True(conn == nil)

	s1.Start()
	defer s1.Stop()
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn != nil) // s1 connected
	is.Equal(conn.GetPort(), 13010)
	listc = l.GetConnections()
	is.Equal(len(listc), 1)
	is.Equal(listc[0].GetPort(), 13010)

	s2.Start()
	defer s2.Stop()
	time.Sleep(2 * time.Second)
	// First connection is still s1.
	conn = l.GetFirstConn()
	is.True(conn != nil)
	is.Equal(conn.GetPort(), 13010)
	// But the second one should be in there.
	listc = l.GetConnections()
	is.Equal(len(listc), 2)
	is.Equal(listc[0].GetPort(), 13010)
	is.Equal(listc[1].GetPort(), 13013)

	s1.Stop()
	time.Sleep(2 * time.Second)
	// Now s2 has become the first.
	conn = l.GetFirstConn()
	is.True(conn != nil)
	is.Equal(conn.GetPort(), 13013)
	listc = l.GetConnections()
	is.Equal(len(listc), 1)
	is.Equal(listc[0].GetPort(), 13013)
}

func TestProjectConnectionScanner(t *testing.T) {
	is := is.New(t)

	instancesDir := t.TempDir()
	rootDir := t.TempDir()

	// A project physically under the root.
	inside := filepath.Join(rootDir, "mygame")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}
	writeInstanceWithProject(t, instancesDir, "inside", 13100, inside)

	// A project outside the root.
	outside := filepath.Join(t.TempDir(), "othergame")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}
	writeInstanceWithProject(t, instancesDir, "outside", 13101, outside)

	// An instance whose project no longer exists on disk is simply excluded.
	writeInstanceWithProject(t, instancesDir, "missing", 13102, filepath.Join(rootDir, "deleted"))

	scanner := &ProjectConnectionScanner{
		InstancesPath: instancesDir,
		GetRootPaths:  func() []string { return []string{rootDir} },
	}

	desired := scanner.Desired()
	is.Equal(len(desired), 1) // only the project under the root
	is.Equal(desired[0].InstanceID, "inside")
}

func TestIsUnderRoot(t *testing.T) {
	is := is.New(t)

	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("creating nested dir: %v", err)
	}

	// The root itself counts as under the root.
	under, err := IsPathUnderRoot(root, root)
	is.NoErr(err)
	is.True(under)

	// A nested directory is under the root.
	under, err = IsPathUnderRoot(nested, root)
	is.NoErr(err)
	is.True(under)

	// A sibling that merely shares a name prefix is not under the root.
	sibling := root + "-sibling"
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("creating sibling dir: %v", err)
	}
	under, err = IsPathUnderRoot(sibling, root)
	is.NoErr(err)
	is.True(!under)

	// A symlink pointing into the root resolves to a path under it.
	t.Run("symlink", func(t *testing.T) {
		is := is.New(t)

		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(nested, link); err != nil {
			// Windows only allows this for an admin or in Developer Mode.
			if runtime.GOOS == "windows" {
				t.Skip("creating a symlink is not permitted:", err)
			}
			t.Fatalf("creating symlink: %v", err)
		}

		under, err := IsPathUnderRoot(link, root)
		is.NoErr(err)
		is.True(under)
	})

	// A non-existent path is an error, not a false "under" result.
	_, err = IsPathUnderRoot(filepath.Join(root, "nope"), root)
	is.True(err != nil)
}
