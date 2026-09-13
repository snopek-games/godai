//go:build !linux

package core

func StartVirtualDisplay(string) (string, error) {
	return "", NewUserError("offscreen editors are only supported on Linux for now", nil, nil)
}
