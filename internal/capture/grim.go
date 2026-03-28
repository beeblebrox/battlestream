package capture

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os/exec"

	"golang.org/x/image/draw"
)

// grimScreenshotter captures screenshots via grim on Wayland/Hyprland.
type grimScreenshotter struct {
	monitor string
	targetW int
	targetH int
}

// NewScreenshotter creates a Screenshotter that uses grim for capture
// and scales output to targetW x targetH.
func NewScreenshotter(monitor string, targetW, targetH int) Screenshotter {
	return &grimScreenshotter{
		monitor: monitor,
		targetW: targetW,
		targetH: targetH,
	}
}

func (g *grimScreenshotter) Capture(ctx context.Context) (image.Image, error) {
	cmd := exec.CommandContext(ctx, "grim", "-o", g.monitor, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("grim: %w: %s", err, stderr.String())
	}

	src, err := png.Decode(&stdout)
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}

	// Scale to target resolution.
	srcBounds := src.Bounds()
	if srcBounds.Dx() == g.targetW && srcBounds.Dy() == g.targetH {
		return src, nil // already correct size
	}

	dst := image.NewRGBA(image.Rect(0, 0, g.targetW, g.targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, draw.Over, nil)
	return dst, nil
}
