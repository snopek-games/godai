package selfupdate

import (
	"fmt"
	"runtime"
)

type Variant struct {
	Name string
	Ext  string
}

// releaseVariants has to agree with the mcp-build matrix in .gitlab-ci.yml,
// which TestReleaseVariantsMatchCIMatrix checks.
var releaseVariants = map[string]Variant{
	"linux/amd64":   {Name: "linux-x86_64", Ext: ""},
	"linux/arm64":   {Name: "linux-arm64", Ext: ""},
	"windows/amd64": {Name: "windows-x86_64", Ext: ".exe"},
	"windows/arm64": {Name: "windows-arm64", Ext: ".exe"},
	"darwin/arm64":  {Name: "macos-arm64", Ext: ""},
}

func VariantForRuntime() (Variant, error) {
	return variantForPlatform(runtime.GOOS, runtime.GOARCH)
}

func variantForPlatform(goos, goarch string) (Variant, error) {
	variant, ok := releaseVariants[goos+"/"+goarch]
	if !ok {
		return Variant{}, fmt.Errorf("no release is built for %s/%s", goos, goarch)
	}
	return variant, nil
}

func variantByName(name string) (Variant, error) {
	for _, variant := range releaseVariants {
		if variant.Name == name {
			return variant, nil
		}
	}
	return Variant{}, fmt.Errorf("unknown release variant %q", name)
}
