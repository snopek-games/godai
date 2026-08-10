package godot

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Discovers which editor instances the manager should connect to.
type ConnectionScanner interface {
	Desired() []instance
}

type ConnectionManagerConfig struct {
	Scanner      ConnectionScanner
	ScanInterval time.Duration
	RetryDelay   time.Duration
	OnConnect    func(*Connection) error
	OnDisconnect func(*Connection)
}

// Connects to every live editor advertised in the global instances directory.
type GlobalConnectionScanner struct {
	InstancesPath string
}

// Connects only to the editors running a known set of projects.
type ProjectConnectionScanner struct {
	InstancesPath string
	GetRootPaths  func() []string
}

type instance struct {
	InstanceID  string `json:"instance_id"`
	PID         int    `json:"pid"`
	ProjectPath string `json:"project_path"`
	Secret      string `json:"secret"`
	Port        int    `json:"port"`
}

type ConnectionManager struct {
	config ConnectionManagerConfig
	doneCh chan struct{}
	done   bool

	// Held for a whole scan: two scans running at once can see different sets
	// of instances, and the slower one would prune the loops the other started.
	scanMutex sync.Mutex

	mutex sync.Mutex
	// maps an instance ID to the stop channel for its connection loop.
	connectionLoops map[string]chan struct{}
}

func NewConnectionManager(config ConnectionManagerConfig) *ConnectionManager {
	return &ConnectionManager{
		config:          config,
		doneCh:          make(chan struct{}),
		connectionLoops: make(map[string]chan struct{}),
	}
}

func (m *ConnectionManager) Start() error {
	if m.done {
		return errors.New("cannot restart a ConnectionManager")
	}
	go m.scanLoop()
	return nil
}

func (m *ConnectionManager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.done {
		m.done = true
		close(m.doneCh)
	}
}

func (m *ConnectionManager) ScanNow() {
	m.mutex.Lock()
	done := m.done
	m.mutex.Unlock()

	if !done {
		m.runScanner()
	}
}

// InstanceProjectPaths returns the project path of every editor instance the
// scanner currently advertises, whether or not we've finished connecting to it.
func (m *ConnectionManager) InstanceProjectPaths() []string {
	desired := m.config.Scanner.Desired()

	paths := make([]string, 0, len(desired))
	for _, inst := range desired {
		if inst.ProjectPath != "" {
			paths = append(paths, inst.ProjectPath)
		}
	}
	return paths
}

const minScanInterval = 100 * time.Millisecond

func (m *ConnectionManager) scanLoop() {
	ticker := time.NewTicker(max(m.config.ScanInterval, minScanInterval))
	defer ticker.Stop()

	for {
		m.runScanner()

		select {
		case <-m.doneCh:
			return
		case <-ticker.C:
		}
	}
}

func (m *ConnectionManager) runScanner() {
	m.scanMutex.Lock()
	defer m.scanMutex.Unlock()

	desired := m.config.Scanner.Desired()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.done {
		return
	}

	// Start a connection loop for any newly-discovered editor.
	seen := make(map[string]bool, len(desired))
	for _, inst := range desired {
		if seen[inst.InstanceID] {
			continue
		}
		seen[inst.InstanceID] = true
		if _, ok := m.connectionLoops[inst.InstanceID]; ok {
			continue
		}
		stopCh := make(chan struct{})
		m.connectionLoops[inst.InstanceID] = stopCh
		go m.connectionLoop(inst, stopCh)
	}

	// Stop the connection loops for editors that are no longer desired.
	for id, stopCh := range m.connectionLoops {
		if !seen[id] {
			close(stopCh)
			delete(m.connectionLoops, id)
		}
	}
}

func (s *GlobalConnectionScanner) Desired() []instance {
	return readAllInstances(s.InstancesPath)
}

func (s *ProjectConnectionScanner) Desired() []instance {
	roots := s.GetRootPaths()

	var instances []instance
	for _, inst := range readAllInstances(s.InstancesPath) {
		if inst.ProjectPath == "" {
			continue
		}
		if IsPathUnderAnyRoot(inst.ProjectPath, roots) {
			instances = append(instances, inst)
		}
	}

	return instances
}

func readAllInstances(instancesPath string) []instance {
	entries, err := os.ReadDir(instancesPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Error("unable to read instances directory", "path", instancesPath, "error", err)
		}
		return nil
	}

	var instances []instance
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if inst, ok := readInstanceFile(filepath.Join(instancesPath, entry.Name())); ok {
			instances = append(instances, inst)
		}
	}

	return instances
}

func readInstanceFile(path string) (instance, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		slog.Error("unable to read instance file", "path", path, "error", err)
		return instance{}, false
	}

	var inst instance
	if err := json.Unmarshal(b, &inst); err != nil {
		slog.Error("unable to parse instance file", "path", path, "error", err)
		return instance{}, false
	}

	if inst.InstanceID == "" || inst.Port == 0 {
		slog.Error("instance file is missing required fields", "path", path)
		return instance{}, false
	}

	// If the editor that wrote this file is no longer running, it exited
	// without cleaning up after itself. Remove the stale file rather than
	// repeatedly trying to connect to a dead editor.
	if !isProcessRunning(inst.PID) {
		slog.Info("removing stale instance file", "path", path, "pid", inst.PID)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Error("unable to remove stale instance file", "path", path, "error", err)
		}
		return instance{}, false
	}

	return inst, true
}

func (m *ConnectionManager) connectionLoop(inst instance, stopCh chan struct{}) {
	instanceID, port := inst.InstanceID, inst.Port
	slog.Debug("starting connection loop", "instance", instanceID, "port", port)

	wsURL := "ws://localhost:" + strconv.Itoa(port) + "?token=" + url.QueryEscape(inst.Secret)
	for {
		select {
		case <-m.doneCh:
			return
		case <-stopCh:
			slog.Debug("stopping connection loop", "instance", instanceID, "port", port)
			return
		default:
		}

		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			slog.Debug("failed to connect", "port", port, "error", err)
			if m.waitBeforeRetry(stopCh) {
				return
			}
			continue
		}

		slog.Info("connected to editor", "port", port)
		conn := NewConnection(ws, port)

		// Start the connection's run loop.
		connErrCh := make(chan error, 1)
		go func() {
			err := conn.Run()
			connErrCh <- err
		}()

		// Do any on connection setup, and discard the connection if there's errors.
		if m.config.OnConnect != nil {
			if err = m.config.OnConnect(conn); err != nil {
				slog.Error("error setting up editor connection", "port", port, "error", err)
				conn.Close()
				<-connErrCh
				if m.waitBeforeRetry(stopCh) {
					return
				}
				continue
			}
		}

		// Wait until the run loop is completed, or we're told to stop.
		select {
		case err = <-connErrCh:
		case <-m.doneCh:
			conn.Close()
			<-connErrCh
		case <-stopCh:
			conn.Close()
			<-connErrCh
		}

		slog.Info("disconnected from editor", "port", port, "error", err)
		if m.config.OnDisconnect != nil {
			m.config.OnDisconnect(conn)
		}

		if m.waitBeforeRetry(stopCh) {
			return
		}
	}
}

// waitBeforeRetry sleeps for RetryDelay, returning true if the manager is
// stopping or this editor's instance file has gone away (in which case the
// caller should stop rather than retry).
func (m *ConnectionManager) waitBeforeRetry(stopCh chan struct{}) bool {
	select {
	case <-m.doneCh:
		return true
	case <-stopCh:
		return true
	case <-time.After(m.config.RetryDelay):
		return false
	}
}
