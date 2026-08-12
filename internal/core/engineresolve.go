package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"

	"gitlab.com/snopek-games/godai/internal/godot"
)

type EngineOptions struct {
	// Version is a version asked for by name, which settles the question
	// rather than being weighed against what the project says.
	Version string
	// AutoInstall downloads a version a project asks for but doesn't have.
	AutoInstall bool
	// Prompt allows asking the user which engine to open a project with, when
	// the project and Godai's own default disagree.
	Prompt bool
}

// errUnknownProjectEngine means the project asks for an engine Godai can't
// work out a version for, so the default is the only thing left to offer.
var errUnknownProjectEngine = errors.New("no build for the version the project asks for")

const (
	answerUpgrade = "upgrade"
	answerPin     = "pin"
)

// GodotExecutable works out which Godot to run for a project, preferring the
// most specific answer: a version asked for by name for this call, then an
// explicit path, then a version asked for by name for this run, then the
// project's own pin, then what project.godot was last saved with, then the
// saved default version, and finally whatever is on $PATH.
func (s *Session) GodotExecutable(ctx context.Context, projectPath string, opts EngineOptions) (string, error) {
	if opts.Version != "" {
		return s.engineExecutable(ctx, opts.Version, opts)
	}

	if path := s.config.GodotPath; path != "" {
		// An unusable path here can come from an ambient $GODOT rather than
		// from anything the user asked for, so it falls through rather than
		// standing in the way.
		if resolved, err := ResolveGodotExecutable(path); err == nil {
			return resolved, nil
		}
		slog.Warn("the Godot executable given by --godot-path or $GODOT can't be run", "godotPath", path)
	}

	if s.config.GodotVersionIsExplicit && s.config.GodotVersion != "" {
		return s.engineExecutable(ctx, s.config.GodotVersion, opts)
	}

	if projectPath != "" {
		pinned, err := ProjectGodotVersion(projectPath)
		if err != nil {
			return "", NewUserError("unable to read "+ProjectConfigName, err, nil)
		}
		if pinned != "" {
			return s.engineExecutable(ctx, pinned, opts)
		}

		path, chosen, err := s.engineForProject(ctx, projectPath, opts)
		if err != nil || chosen {
			return path, err
		}
	}

	if version := s.config.GodotVersion; version != "" {
		return s.engineExecutable(ctx, version, EngineOptions{})
	}

	if path := godotOnPath(); path != "" {
		slog.Debug("no Godot version is configured, so using the one on $PATH", "godotPath", path)
		return path, nil
	}

	return "", NewUserError("no version of Godot is installed or configured", ErrNotConfigured, []string{
		"See what there is: `godai engine search`",
		"Install one: `godai engine install <VERSION>`",
		"Then make it the default: `godai engine use <VERSION>`",
	})
}

// engineForProject chooses an engine from what project.godot was last saved
// with. It reports false when the project settles nothing, leaving the rest of
// the chain to answer.
func (s *Session) engineForProject(ctx context.Context, projectPath string, opts EngineOptions) (string, bool, error) {
	project, err := godot.ProjectFromPath(projectPath)
	if err != nil {
		return "", false, nil
	}

	wanted, named, err := project.GetEngine()
	if err != nil {
		slog.Warn("unable to read the engine version from project.godot", "project", projectPath, "error", err)
		named = false
	}

	preferred := s.preferredEngine()

	if !named {
		return s.openWithPreferred(ctx, preferred, opts,
			"Godai can't tell which version of Godot this project was made with.")
	}

	// A linked engine has to be explicitly selected by the user, so we go with that.
	if preferred != nil && preferred.Linked {
		return preferred.Path, true, nil
	}

	// The default is already a build of what the project asks for.
	if preferred != nil && wanted.Major == preferred.Version.Major && wanted.Minor == preferred.Version.Minor {
		path, err := s.engineExecutable(ctx, withMono(preferred.Version, wanted.Mono).String(), opts)
		return path, true, err
	}

	candidate, err := s.projectEngineVersion(ctx, wanted, opts)
	if err != nil {
		if !errors.Is(err, errUnknownProjectEngine) {
			return "", true, err
		}
		return s.openWithPreferred(ctx, preferred, opts,
			fmt.Sprintf("This project was made with Godot %s, which Godai can't get a build of.", wanted))
	}

	// Opening a project with an older engine than it was made with doesn't
	// work, so a project ahead of the default is never questioned.
	if preferred == nil || candidate.Compare(preferred.Version) > 0 {
		path, err := s.engineExecutable(ctx, candidate.String(), opts)
		return path, true, err
	}

	return s.upgradeOrPin(ctx, projectPath, wanted, candidate, preferred, opts)
}

func (s *Session) projectEngineVersion(ctx context.Context, wanted godot.ProjectEngine, opts EngineOptions) (godot.EngineVersion, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return godot.EngineVersion{}, err
	}

	engine, found, err := manager.FindForProject(wanted)
	if err != nil {
		return godot.EngineVersion{}, err
	}
	if found {
		return engine.Version, nil
	}

	if !opts.AutoInstall {
		return godot.EngineVersion{}, NewUserError(
			fmt.Sprintf("this project was made with Godot %s, which isn't installed", wanted),
			errors.Join(godot.ErrEngineNotInstalled, ErrNotConfigured), []string{
				fmt.Sprintf("Install it: `godai engine install %d.%d`", wanted.Major, wanted.Minor),
				"Or open the project, which installs it: `godai project open`",
			})
	}

	version, err := manager.LatestForProject(ctx, wanted)
	if err != nil {
		slog.Warn("unable to find a release of the version the project asks for", "version", wanted, "error", err)
		return godot.EngineVersion{}, errUnknownProjectEngine
	}

	return version, nil
}

// upgradeOrPin asks what to do about a project made with an older engine than
// the default, since opening it with a newer one changes project.godot.
func (s *Session) upgradeOrPin(ctx context.Context, projectPath string, wanted godot.ProjectEngine, candidate godot.EngineVersion, preferred *godot.Engine, opts EngineOptions) (string, bool, error) {
	useProject := func() (string, bool, error) {
		path, err := s.engineExecutable(ctx, candidate.String(), opts)
		return path, true, err
	}

	if !opts.Prompt {
		return useProject()
	}

	answer, err := s.askChoice(ctx,
		fmt.Sprintf("This project was made with Godot %s, but Godai's default is %s.", wanted, preferred.Name),
		"open_with",
		fmt.Sprintf("Open it with %s, which will update the project to that version (%q), or keep using %s for this project (%q)",
			preferred.Name, answerUpgrade, candidate, answerPin),
		[]string{answerUpgrade, answerPin})
	if err != nil {
		// With no way to ask, the project's own version is the answer that
		// changes nothing.
		slog.Info("opening the project with the version it was made with", "version", candidate, "default", preferred.Name)
		return useProject()
	}

	if answer == answerUpgrade {
		path, err := s.engineExecutable(ctx, withMono(preferred.Version, wanted.Mono).String(), opts)
		return path, true, err
	}

	if err := SetProjectGodotVersion(projectPath, candidate.String()); err != nil {
		return "", true, NewUserError("unable to write "+ProjectConfigName, err, nil)
	}
	return useProject()
}

func (s *Session) openWithPreferred(ctx context.Context, preferred *godot.Engine, opts EngineOptions, why string) (string, bool, error) {
	if preferred == nil {
		return "", false, nil
	}
	if !opts.Prompt {
		return preferred.Path, true, nil
	}

	ok, err := s.askYesNo(ctx, why, "open_with_default",
		fmt.Sprintf("Open it with Godot %s?", preferred.Name))
	if err != nil {
		// Refusing to open a project because nobody can answer would be worse
		// than opening it with the only engine there is.
		slog.Warn("opening the project with the default version of Godot", "version", preferred.Name, "reason", why)
		return preferred.Path, true, nil
	}
	if !ok {
		return "", true, NewUserError("no version of Godot was chosen to open the project with", ErrNotConfigured, []string{
			"Pin one to the project: `godai project pin-engine <VERSION>`",
		})
	}

	return preferred.Path, true, nil
}

func (s *Session) preferredEngine() *godot.Engine {
	if s.config.GodotVersion == "" {
		return nil
	}

	engine, err := s.FindEngine(s.config.GodotVersion)
	if err != nil {
		slog.Warn("the configured version of Godot is unusable", "version", s.config.GodotVersion, "error", err)
		return nil
	}
	return engine
}

func (s *Session) engineExecutable(ctx context.Context, version string, opts EngineOptions) (string, error) {
	engine, err := s.FindEngine(version)
	if err != nil {
		if !opts.AutoInstall || !errors.Is(err, godot.ErrEngineNotInstalled) {
			return "", err
		}
		if engine, err = s.installEngine(ctx, version); err != nil {
			return "", err
		}
	}

	if err := checkExecutable(engine.Path); err != nil {
		return "", NewUserError(fmt.Sprintf("the Godot executable for %s can't be run", engine.Name), err, []string{
			"Install it again: `godai engine install " + engine.Name + "`",
		})
	}

	return engine.Path, nil
}

func (s *Session) installEngine(ctx context.Context, name string) (*godot.Engine, error) {
	version, err := godot.ParseEngineVersion(name)
	if err != nil {
		return nil, engineUserError(name, err)
	}

	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	slog.Info("installing Godot", "version", version)

	engine, err := manager.Install(ctx, version, godot.DownloadOptions{
		Progress: s.startInstallReport(version.String()),
	})
	if err != nil {
		return nil, NewUserError(fmt.Sprintf("unable to install Godot %s", version), err, []string{
			"Check what there is: `godai engine search --all`",
			"Or install it yourself: `godai engine install " + version.String() + "`",
		})
	}

	return engine, nil
}

func engineUserError(version string, err error) error {
	var versionErr *godot.VersionError

	switch {
	case errors.Is(err, godot.ErrEngineNotInstalled):
		return NewUserError(fmt.Sprintf("Godot %s isn't installed", version), errors.Join(err, ErrNotConfigured), []string{
			"Install it: `godai engine install " + version + "`",
			"Or see what is installed: `godai engine list`",
		})
	case errors.As(err, &versionErr):
		return NewUserError(err.Error(), err, []string{
			"Versions look like `4.5`, `4.4.1`, `4.6-beta3` or `4.5-mono`",
			"A linked engine is named by whatever `godai engine link` called it",
		})
	default:
		return NewUserError(fmt.Sprintf("unable to use Godot %s", version), err, nil)
	}
}

func withMono(version godot.EngineVersion, mono bool) godot.EngineVersion {
	version.Mono = mono
	return version
}

func godotOnPath() string {
	for _, name := range []string{"godot4", "godot"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func (s *Session) askChoice(ctx context.Context, message, field, description string, options []string) (string, error) {
	allowed := make([]any, 0, len(options))
	for _, option := range options {
		allowed = append(allowed, option)
	}

	answers, err := s.getPrompter().Prompt(ctx, message, map[string]any{
		"type":     "object",
		"required": []any{field},
		"properties": map[string]any{
			field: map[string]any{
				"type":        "string",
				"description": description,
				"enum":        allowed,
			},
		},
	})
	if err != nil {
		return "", err
	}

	answer, _ := answers[field].(string)
	if answer == "" {
		return "", ErrPromptDeclined
	}
	return answer, nil
}

func (s *Session) askYesNo(ctx context.Context, message, field, description string) (bool, error) {
	answers, err := s.getPrompter().Prompt(ctx, message, map[string]any{
		"type":     "object",
		"required": []any{field},
		"properties": map[string]any{
			field: map[string]any{
				"type":        "boolean",
				"description": description,
				"default":     true,
			},
		},
	})
	if err != nil {
		return false, err
	}

	answer, ok := answers[field].(bool)
	if !ok {
		return false, ErrPromptDeclined
	}
	return answer, nil
}
