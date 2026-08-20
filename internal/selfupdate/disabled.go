//go:build !selfupdate

package selfupdate

// Only official builds pass -tags selfupdate. Custom and go-install builds
// lack it, so godai never replaces a binary it didn't publish.
const Enabled = false
