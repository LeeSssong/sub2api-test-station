package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// One browser at a time per process, in addition to the existing schedule leases.
var drawingRenderSlot = make(chan struct{}, 1)

func drawingRenderDocument(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > 2<<20 {
		return "", fmt.Errorf("invalid artwork size")
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}
	hasSVG := false
	var clean func(*html.Node)
	clean = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode {
				switch strings.ToLower(child.Data) {
				case "script", "iframe", "frame", "frameset", "object", "embed", "meta", "base", "link", "form", "audio", "video", "source":
					n.RemoveChild(child)
					child = next
					continue
				}
				if child.Data == "svg" {
					hasSVG = true
				}
				attrs := child.Attr[:0]
				for _, a := range child.Attr {
					key := strings.ToLower(a.Key)
					if strings.HasPrefix(key, "on") || key == "srcdoc" || key == "nonce" {
						continue
					}
					if (key == "href" || key == "src") && !strings.HasPrefix(a.Val, "#") {
						continue
					}
					attrs = append(attrs, a)
				}
				child.Attr = attrs
			}
			clean(child)
			child = next
		}
	}
	clean(doc)
	if !hasSVG {
		return "", fmt.Errorf("no inline SVG")
	}
	var b bytes.Buffer
	if err := html.Render(&b, doc); err != nil {
		return "", err
	}
	// CSP precedes all authored markup. Browser networking is also disabled via
	// host resolution and offline proxy, and authored scripts never execute.
	return `<!doctype html><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src 'none'; font-src 'none'; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"><style>html,body{margin:0!important;width:100%!important;height:100%!important;overflow:hidden!important}body{display:flex!important;align-items:center!important;justify-content:center!important}svg{max-width:100vw!important;max-height:100vh!important}</style>` + b.String(), nil
}

func renderIntelligenceDrawing(ctx context.Context, raw string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case drawingRenderSlot <- struct{}{}:
		defer func() { <-drawingRenderSlot }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	doc, err := drawingRenderDocument(raw)
	if err != nil {
		return "", err
	}
	binary := os.Getenv("INTELLIGENCE_CHROMIUM_PATH")
	if binary == "" {
		for _, name := range []string{"chromium", "chromium-browser", "google-chrome"} {
			if p, e := exec.LookPath(name); e == nil {
				binary = p
				break
			}
		}
	}
	if binary == "" {
		return "", fmt.Errorf("chromium unavailable")
	}
	dir, err := os.MkdirTemp("", "intelligence-drawing-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	page := filepath.Join(dir, "drawing.html")
	output := filepath.Join(dir, "drawing.png")
	if err = os.WriteFile(page, []byte(doc), 0600); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, binary, "--headless", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-extensions", "--disable-sync", "--disable-javascript", "--hide-scrollbars", "--host-resolver-rules=MAP * ~NOTFOUND", "--proxy-server=http://127.0.0.1:9", "--proxy-bypass-list=<-loopback>", "--user-data-dir="+filepath.Join(dir, "profile"), "--window-size=1024,768", "--force-device-scale-factor=1", "--virtual-time-budget=1000", "--screenshot="+output, "file://"+page)
	// No inherited application credentials. Never disable Chromium's sandbox.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir, "LANG=C.UTF-8"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	// Some Chromium builds finish the PNG then exit nonzero during teardown.
	// Accept only a complete, dimension-checked PNG below, never a timeout.
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if runErr != nil {
		if _, statErr := os.Stat(output); statErr != nil {
			return "", fmt.Errorf("chromium screenshot failed")
		}
	}
	info, err := os.Stat(output)
	if err != nil || info.Size() > 8<<20 {
		return "", fmt.Errorf("invalid screenshot")
	}
	rawPNG, err := os.ReadFile(output)
	if err != nil {
		return "", err
	}
	config, err := png.DecodeConfig(bytes.NewReader(rawPNG))
	if err != nil || config.Width != 1024 || config.Height != 768 {
		return "", fmt.Errorf("unexpected screenshot dimensions")
	}
	src, err := png.Decode(bytes.NewReader(rawPNG))
	if err != nil {
		return "", err
	}
	small := image.NewRGBA(image.Rect(0, 0, 768, 576))
	for y := 0; y < 576; y++ {
		for x := 0; x < 768; x++ {
			small.Set(x, y, src.At(x*1024/768, y*768/576))
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, small); err != nil {
		return "", err
	}
	if encoded.Len() > 1<<20 {
		return "", fmt.Errorf("screenshot too large")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
