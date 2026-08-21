package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/matryer/is"
)

type stubPrompter struct {
	answers  map[string]any
	err      error
	messages []string
}

func (p *stubPrompter) Prompt(_ context.Context, message string, _ map[string]any) (map[string]any, error) {
	p.messages = append(p.messages, message)
	return p.answers, p.err
}

// installEngineFor puts an engine in the cache the way a real install would,
// so that resolving one doesn't need the network.
func installEngineFor(t *testing.T, version string) string {
	t.Helper()

	cachePath, err := GetCachePath()
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(cachePath, "engines", version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	executable := filepath.Join(dir, "godot")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	record := fmt.Sprintf(`{"version":%q,"executable":"godot"}`, version)
	if err := os.WriteFile(filepath.Join(dir, "godai-engine.json"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}

	return executable
}

func projectWithFeatures(t *testing.T, features string) string {
	t.Helper()

	dir := t.TempDir()
	contents := "config_version=5\n\n[application]\n\nconfig/name=\"Test\"\n"
	if features != "" {
		contents += "config/features=" + features + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func engineSession(t *testing.T, godotVersion string) *Session {
	t.Helper()
	return engineSessionFor(t, godotVersion, false)
}

func engineSessionFor(t *testing.T, godotVersion string, explicit bool) *Session {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("relies on the XDG paths and on a shell script standing in for Godot")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))

	session, err := New(Config{
		SavedConfigPath:        filepath.Join(dir, "config", "godai", "config.json"),
		GodotVersion:           godotVersion,
		GodotVersionIsExplicit: explicit,
		NoAutoInstall:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)

	return session
}

func resolve(t *testing.T, session *Session, projectPath string) (string, error) {
	t.Helper()
	return session.GodotExecutable(t.Context(), projectPath, EngineOptions{Prompt: true})
}

func TestTheDefaultIsUsedWhenItIsABuildOfWhatTheProjectAsksFor(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.5.1-stable")
	installEngineFor(t, "4.5-stable")
	wanted := installEngineFor(t, "4.5.1-stable")

	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.5", "GL Compatibility")`))
	is.NoErr(err)
	is.Equal(path, wanted)
}

func TestAProjectNewerThanTheDefaultIsOpenedWithoutAsking(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.5-stable")
	installEngineFor(t, "4.5-stable")
	wanted := installEngineFor(t, "4.7-stable")

	prompter := &stubPrompter{}
	session.SetPrompter(prompter)

	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.7")`))
	is.NoErr(err)
	is.Equal(path, wanted)
	is.Equal(len(prompter.messages), 0) // nothing to ask about
}

func TestTheNewestInstalledBuildOfTheProjectVersionWins(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "")
	installEngineFor(t, "4.7-stable")
	installEngineFor(t, "4.7.1-rc1")
	wanted := installEngineFor(t, "4.7.1-stable")

	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.7")`))
	is.NoErr(err)
	is.Equal(path, wanted)
}

func TestAProjectOlderThanTheDefaultAsksWhatToDo(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.7-stable")
	newer := installEngineFor(t, "4.7-stable")
	older := installEngineFor(t, "4.4-stable")
	projectPath := projectWithFeatures(t, `PackedStringArray("4.4")`)

	prompter := &stubPrompter{answers: map[string]any{"open_with": answerUpgrade}}
	session.SetPrompter(prompter)

	path, err := resolve(t, session, projectPath)
	is.NoErr(err)
	is.Equal(path, newer)
	is.Equal(len(prompter.messages), 1)
	is.True(strings.Contains(prompter.messages[0], "4.4"))

	// Upgrading is for this run only, so the project isn't changed.
	pinned, err := ProjectGodotVersion(projectPath)
	is.NoErr(err)
	is.Equal(pinned, "")

	session.SetPrompter(&stubPrompter{answers: map[string]any{"open_with": answerPin}})

	path, err = resolve(t, session, projectPath)
	is.NoErr(err)
	is.Equal(path, older)

	pinned, err = ProjectGodotVersion(projectPath)
	is.NoErr(err)
	is.Equal(pinned, "4.4-stable")
}

func TestAProjectOlderThanTheDefaultIsLeftAloneWhenNobodyCanAnswer(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.7-stable")
	installEngineFor(t, "4.7-stable")
	older := installEngineFor(t, "4.4-stable")

	// The session starts with NoPrompter, which is what --no-input and MCP
	// clients without elicitation leave it as.
	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.4")`))
	is.NoErr(err)
	is.Equal(path, older) // never silently upgrades the project
}

func TestACSharpProjectGetsTheMonoBuild(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.7-stable")
	installEngineFor(t, "4.7-stable")
	wanted := installEngineFor(t, "4.7-stable-mono")

	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.7", "C#", "Forward Plus")`))
	is.NoErr(err)
	is.Equal(path, wanted)
}

func TestACSharpProjectIsFoundByItsCsprojToo(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "")
	wanted := installEngineFor(t, "4.7-stable-mono")

	projectPath := projectWithFeatures(t, `PackedStringArray("4.7")`)
	is.NoErr(os.WriteFile(filepath.Join(projectPath, "Test.csproj"), []byte("<Project/>"), 0o644))

	path, err := resolve(t, session, projectPath)
	is.NoErr(err)
	is.Equal(path, wanted)
}

func TestAProjectNamingNoVersionAsksAboutTheDefault(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.7-stable")
	wanted := installEngineFor(t, "4.7-stable")

	prompter := &stubPrompter{answers: map[string]any{"open_with_default": true}}
	session.SetPrompter(prompter)

	// A project last saved by Godot 3 records no features at all.
	path, err := resolve(t, session, projectWithFeatures(t, ""))
	is.NoErr(err)
	is.Equal(path, wanted)
	is.Equal(len(prompter.messages), 1)

	session.SetPrompter(&stubPrompter{answers: map[string]any{"open_with_default": false}})

	_, err = resolve(t, session, projectWithFeatures(t, ""))
	is.True(err != nil) // saying no means there's nothing to open it with
}

func TestAPinBeatsWhatTheProjectWasSavedWith(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.7-stable")
	installEngineFor(t, "4.7-stable")
	wanted := installEngineFor(t, "4.4-stable")

	projectPath := projectWithFeatures(t, `PackedStringArray("4.7")`)
	is.NoErr(SetProjectGodotVersion(projectPath, "4.4-stable"))

	prompter := &stubPrompter{}
	session.SetPrompter(prompter)

	path, err := resolve(t, session, projectPath)
	is.NoErr(err)
	is.Equal(path, wanted)
	is.Equal(len(prompter.messages), 0) // a pin settles it
}

func TestALinkedDefaultIsTakenAtItsWord(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "my-build")
	installEngineFor(t, "4.7-stable")

	// A linked engine is whatever the user pointed Godai at, so it isn't
	// compared with what the project asks for.
	linked := installEngineFor(t, "4.4-stable")
	manager, err := session.EngineManager()
	is.NoErr(err)
	_, err = manager.Link(context.Background(), "my-build", linked)
	is.NoErr(err)

	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.7")`))
	is.NoErr(err)
	is.Equal(path, linked)
}

func TestAVersionAskedForByNameBeatsThePinAndTheProject(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.4-stable")
	installEngineFor(t, "4.4-stable")
	wanted := installEngineFor(t, "4.7-stable")

	projectPath := projectWithFeatures(t, `PackedStringArray("4.4")`)
	is.NoErr(SetProjectGodotVersion(projectPath, "4.4-stable"))

	path, err := session.GodotExecutable(t.Context(), projectPath, EngineOptions{Version: "4.7-stable"})
	is.NoErr(err)
	is.Equal(path, wanted)

	_, err = session.GodotExecutable(t.Context(), projectPath, EngineOptions{Version: "not-a-build"})
	is.True(err != nil) // a name that isn't an engine isn't quietly ignored
}

func TestAVersionGivenForTheRunBeatsThePinAndTheProject(t *testing.T) {
	is := is.New(t)

	session := engineSessionFor(t, "4.4-stable", true)
	wanted := installEngineFor(t, "4.4-stable")
	installEngineFor(t, "4.7-stable")

	projectPath := projectWithFeatures(t, `PackedStringArray("4.7")`)
	is.NoErr(SetProjectGodotVersion(projectPath, "4.7-stable"))

	path, err := resolve(t, session, projectPath)
	is.NoErr(err)
	is.Equal(path, wanted)
}

// The saved default is the flag's value when it isn't given, so treating it as
// a version asked for by name would open every project with it.
func TestTheSavedDefaultIsStillWeighedAgainstTheProject(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.4-stable")
	installEngineFor(t, "4.4-stable")
	wanted := installEngineFor(t, "4.7-stable")

	path, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.7")`))
	is.NoErr(err)
	is.Equal(path, wanted)
}

func TestAnUninstalledProjectVersionIsReportedRatherThanIgnored(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "4.5-stable")
	installEngineFor(t, "4.5-stable")

	_, err := resolve(t, session, projectWithFeatures(t, `PackedStringArray("4.7")`))
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "4.7")) // the error names the uninstalled version
}
