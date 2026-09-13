package addon

import (
	"testing"
)

// The addon suite's editor is headless, so there is nothing to render. The
// offscreen suite covers an actual capture.
func TestTakeScreenshotFailsHeadless(t *testing.T) {
	callToolErr(t, "take_screenshot", map[string]any{}, "headless")
}
