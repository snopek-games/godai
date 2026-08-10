package core

import (
	"context"
	"gitlab.com/snopek-games/godai/internal/godot"
	"testing"
	"time"

	"github.com/matryer/is"
)

func (s *Session) addEditor(projectPath string, conn *godot.Connection) {
	s.addEditorWithHeadless(projectPath, conn, false)
}

func (s *Session) addEditorWithHeadless(projectPath string, conn *godot.Connection, headless bool) {
	s.editorsMutex.Lock()
	defer s.editorsMutex.Unlock()
	s.editors = append(s.editors, &Editor{ProjectPath: projectPath, conn: conn, Headless: headless})
	s.notifyEditorsChanged()
}

func newTestSession(t *testing.T) *Session {
	t.Helper()
	s, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWaitForEditorReconnect(t *testing.T) {
	const projectPath = "/some/project"

	t.Run("reconnects_with_new_connection", func(t *testing.T) {
		is := is.New(t)

		s := newTestSession(t)
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

		editor, err := s.waitForReconnect(ctx, projectPath, oldConn)
		is.NoErr(err)
		is.Equal(editor.conn, newConn)
	})

	t.Run("ignores_the_old_connection_lingering", func(t *testing.T) {
		is := is.New(t)

		// If the old connection is still present (hasn't dropped yet), we must
		// keep waiting rather than returning it.
		s := newTestSession(t)
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := s.waitForReconnect(ctx, projectPath, oldConn)
		is.True(err != nil) // timed out, since only the old connection exists
	})

	t.Run("times_out_when_no_editor_returns", func(t *testing.T) {
		is := is.New(t)

		s := newTestSession(t)
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)
		s.onEditorDisconnect(oldConn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := s.waitForReconnect(ctx, projectPath, oldConn)
		is.True(err != nil)
	})
}

func TestWaitForEditorDisconnect(t *testing.T) {
	const projectPath = "/some/project"

	t.Run("returns_when_connection_drops", func(t *testing.T) {
		is := is.New(t)

		s := newTestSession(t)
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)

		go func() {
			time.Sleep(300 * time.Millisecond)
			s.onEditorDisconnect(oldConn)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := s.waitForDisconnect(ctx, projectPath, oldConn)
		is.NoErr(err)
	})

	t.Run("returns_when_replaced_by_a_new_connection", func(t *testing.T) {
		is := is.New(t)

		// A fresh connection for the same project also means the old one is gone.
		s := newTestSession(t)
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

		err := s.waitForDisconnect(ctx, projectPath, oldConn)
		is.NoErr(err)
	})

	t.Run("times_out_while_connection_lingers", func(t *testing.T) {
		is := is.New(t)

		s := newTestSession(t)
		oldConn := godot.NewConnection(nil, 1)
		s.addEditor(projectPath, oldConn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := s.waitForDisconnect(ctx, projectPath, oldConn)
		is.True(err != nil) // timed out, since the connection never dropped
	})
}
