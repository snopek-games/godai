package offscreen

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestTakeScreenshotReturnsAPNGOfTheEditor(t *testing.T) {
	is := is.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := client.CallTool(ctx, "take_screenshot", map[string]any{})
	is.NoErr(err)
	is.True(!result.IsError) // the capture succeeded

	structured, err := result.Structured()
	is.NoErr(err)
	is.Equal(structured["success"], true)

	project, err := client.CallTool(ctx, "get_current_project", map[string]any{})
	is.NoErr(err)
	projectInfo, err := project.Structured()
	is.NoErr(err)
	is.Equal(projectInfo["offscreen"], true) // the editor is rendering to the virtual display

	var images int
	for _, item := range result.Content {
		if item.Type != "image" {
			continue
		}
		images++
		is.Equal(item.MimeType, "image/png")

		data, err := base64.StdEncoding.DecodeString(item.Data)
		is.NoErr(err)
		decoded, err := png.Decode(bytes.NewReader(data))
		is.NoErr(err)

		bounds := decoded.Bounds()
		is.True(bounds.Dx() > 0 && bounds.Dy() > 0)
		is.True(bounds.Dx() <= displayWidth && bounds.Dy() <= displayHeight) // no bigger than the display it draws to
	}
	is.Equal(images, 1)
}
