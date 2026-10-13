package service

import (
	"context"
	"encoding/base64"
	"github.com/stretchr/testify/require"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDrawingRenderDocumentBlocksActiveContent(t *testing.T) {
	doc, err := drawingRenderDocument(`<html><head><meta http-equiv="refresh" content="0;url=http://localhost"><script>fetch('/secret')</script></head><body onload="evil()"><iframe src="file:///etc/passwd"></iframe><svg viewBox="0 0 100 100"><rect width="100" height="100" fill="red"/></svg></body></html>`)
	require.NoError(t, err)
	require.NotContains(t, doc, "fetch(")
	require.NotContains(t, doc, "onload")
	require.NotContains(t, doc, "iframe")
	require.NotContains(t, doc, "http://localhost")
	require.Contains(t, doc, "script-src 'none'")
	require.Contains(t, doc, `fill="red"`)
	_, err = drawingRenderDocument("plain text")
	require.Error(t, err)
}
func TestDrawingRenderChromium(t *testing.T) {
	if os.Getenv("INTELLIGENCE_CHROMIUM_PATH") == "" {
		t.Skip("explicit local browser required")
	}
	data, err := renderIntelligenceDrawing(context.Background(), `<html><body style="margin:0"><svg viewBox="0 0 1024 768"><rect width="1024" height="768" fill="#e03020"/></svg></body></html>`)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	require.NoError(t, err)
	img, err := png.Decode(strings.NewReader(string(raw)))
	require.NoError(t, err)
	require.Equal(t, 768, img.Bounds().Dx())
	require.Equal(t, 576, img.Bounds().Dy())
	r, g, b, _ := img.At(384, 288).RGBA()
	require.Greater(t, r, g*2)
	require.Greater(t, r, b*2)
}

func TestDrawingRenderAcceptsValidatedPNGAfterTeardownFailure(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.png")
	f, err := os.Create(source)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1024, 768))))
	require.NoError(t, f.Close())
	binary := filepath.Join(dir, "chromium-fixture")
	script := "#!/bin/sh\nfor arg in \"$@\"; do case \"$arg\" in --screenshot=*) out=${arg#--screenshot=};; esac; done\ncp '" + strings.ReplaceAll(source, "'", "'\\''") + "' \"$out\"\nexit 2\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0700))
	t.Setenv("INTELLIGENCE_CHROMIUM_PATH", binary)
	_, err = renderIntelligenceDrawing(context.Background(), "<svg/>")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte("truncated PNG"), 0600))
	_, err = renderIntelligenceDrawing(context.Background(), "<svg/>")
	require.Error(t, err)
}
