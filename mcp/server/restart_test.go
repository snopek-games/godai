package server

import (
	"context"
	"godai/mcp/godot"
	"testing"
	"time"

	"github.com/matryer/is"
)

// addEditor and dropEditor mimic what the connection-manager callbacks do to
// the server's editor list, so we can drive waitForEditorReconnect in a test.
func (s *Server) addEditor(projectPath string, conn *godot.Connection) {
	s.editorsMutex.Lock()
	defer s.editorsMutex.Unlock()
	s.editors = append(s.editors, &editorInfo{ProjectPath: projectPath, Connection: conn})
}

func TestWaitForEditorReconnect(t *testing.T) {
	const projectPath = "/some/project"

	t.Run("reconnects_with_new_connection", func(t *testing.T) {
		is := is.New(t)

		s := NewServer(&Config{})
		oldConn := godot.NewConnection(nil, 1)
		newConn := godot.NewConnection(nil, 2)
		s.addEditor(projectPath, oldConn)

		go func() {
			// Old editor goes away...
			time.Sleep(300 * time.Millisecond)
			s.onEditorDisconnect(oldConn)
			// ...then a fresh one connects back.
			time.Sleep(300 * time.Millisecond)
			s.addEditor(projectPath, newConn)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := s.waitForEditorReconnect(ctx, projectPath, oldConn)
		is.NoErr(err)
		is.Equal(conn, newConn)
	})

	t.Run("ignores_the_old_connection_lingering", func(t *testing.T) {
		is := is.New(t)

		// If the old connection is still present (hasn't dropped yet), we must
		// keep waiting rather than returning it.
		s := NewServer(&Config{})
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := s.waitForEditorReconnect(ctx, projectPath, oldConn)
		is.True(err != nil) // timed out, since only the old connection exists
	})

	t.Run("times_out_when_no_editor_returns", func(t *testing.T) {
		is := is.New(t)

		s := NewServer(&Config{})
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)
		s.onEditorDisconnect(oldConn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := s.waitForEditorReconnect(ctx, projectPath, oldConn)
		is.True(err != nil)
	})
}

func TestWaitForEditorDisconnect(t *testing.T) {
	const projectPath = "/some/project"

	t.Run("returns_when_connection_drops", func(t *testing.T) {
		is := is.New(t)

		s := NewServer(&Config{})
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)

		go func() {
			time.Sleep(300 * time.Millisecond)
			s.onEditorDisconnect(oldConn)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := s.waitForEditorDisconnect(ctx, projectPath, oldConn)
		is.NoErr(err)
	})

	t.Run("returns_when_replaced_by_a_new_connection", func(t *testing.T) {
		is := is.New(t)

		// A fresh connection for the same project also means the old one is gone.
		s := NewServer(&Config{})
		oldConn := godot.NewConnection(nil, 1)
		newConn := godot.NewConnection(nil, 2)
		s.addEditor(projectPath, oldConn)

		go func() {
			time.Sleep(300 * time.Millisecond)
			s.onEditorDisconnect(oldConn)
			s.addEditor(projectPath, newConn)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := s.waitForEditorDisconnect(ctx, projectPath, oldConn)
		is.NoErr(err)
	})

	t.Run("times_out_while_connection_lingers", func(t *testing.T) {
		is := is.New(t)

		s := NewServer(&Config{})
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := s.waitForEditorDisconnect(ctx, projectPath, oldConn)
		is.True(err != nil) // timed out, since the connection never dropped
	})
}
