//go:build !linux

package core

func startVirtualDisplay(string) (string, error) {
	return "", NewUserError("offscreen editors are only supported on Linux for now", nil, nil)
}
