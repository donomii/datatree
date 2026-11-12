package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func encodeIconPNG(size int) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) * 0.22
	margin := float64(size) * 0.06
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(x)
			fy := float64(y)
			if insideRoundedRect(fx, fy, margin, margin, float64(size)-2*margin, float64(size)-2*margin, radius) {
				t := fy / float64(size)
				r := uint8(18 + t*14)
				g := uint8(66 + t*34)
				b := uint8(126 + t*36)
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
			}
		}
	}

	s := float64(size)
	lineColor := color.RGBA{R: 226, G: 240, B: 255, A: 255}
	drawSegment(img, s*0.26, s*0.28, s*0.26, s*0.74, s*0.032, lineColor)
	drawSegment(img, s*0.26, s*0.36, s*0.64, s*0.36, s*0.028, lineColor)
	drawSegment(img, s*0.26, s*0.54, s*0.56, s*0.54, s*0.028, lineColor)
	drawSegment(img, s*0.26, s*0.72, s*0.68, s*0.72, s*0.028, lineColor)

	drawDisc(img, s*0.26, s*0.26, s*0.075, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	drawDisc(img, s*0.68, s*0.36, s*0.065, color.RGBA{R: 110, G: 231, B: 183, A: 255})
	drawDisc(img, s*0.60, s*0.54, s*0.065, color.RGBA{R: 253, G: 224, B: 71, A: 255})
	drawDisc(img, s*0.72, s*0.72, s*0.065, color.RGBA{R: 147, G: 197, B: 253, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	} else {
		return buf.Bytes(), nil
	}
}

func insideRoundedRect(px, py, rx, ry, rw, rh, rad float64) bool {
	if px < rx || px > rx+rw || py < ry || py > ry+rh {
		return false
	} else {
		cx := math.Max(rx+rad, math.Min(px, rx+rw-rad))
		cy := math.Max(ry+rad, math.Min(py, ry+rh-rad))
		dx := px - cx
		dy := py - cy
		return dx*dx+dy*dy <= rad*rad
	}
}

func drawDisc(img *image.RGBA, cx, cy, rad float64, c color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			if dx*dx+dy*dy <= rad*rad {
				img.SetRGBA(x, y, c)
			} else {
				// Outside disc radius.
			}
		}
	}
}

func drawSegment(img *image.RGBA, x1, y1, x2, y2, halfWidth float64, c color.RGBA) {
	b := img.Bounds()
	vx := x2 - x1
	vy := y2 - y1
	segLenSq := vx*vx + vy*vy
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			t := 0.0
			if segLenSq > 0 {
				t = ((float64(x)-x1)*vx + (float64(y)-y1)*vy) / segLenSq
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				} else {
					// Parameter stays inside segment bounds.
				}
			} else {
				// Degenerate zero-length segment.
			}
			px := x1 + t*vx
			py := y1 + t*vy
			dx := float64(x) - px
			dy := float64(y) - py
			if dx*dx+dy*dy <= halfWidth*halfWidth {
				img.SetRGBA(x, y, c)
			} else {
				// Outside stroke thickness.
			}
		}
	}
}

func buildICNS() ([]byte, error) {
	chunks := []struct {
		tag  string
		size int
	}{
		{"ic08", 256},
		{"ic09", 512},
	}
	var body bytes.Buffer
	for _, chunk := range chunks {
		pngBytes, err := encodeIconPNG(chunk.size)
		if err != nil {
			return nil, fmt.Errorf("cannot encode %dx%d icon PNG: %v", chunk.size, chunk.size, err)
		} else {
			var header [8]byte
			copy(header[:4], []byte(chunk.tag))
			binary.BigEndian.PutUint32(header[4:], uint32(8+len(pngBytes)))
			_, _ = body.Write(header[:])
			_, _ = body.Write(pngBytes)
		}
	}
	var rootHeader [8]byte
	copy(rootHeader[:4], []byte("icns"))
	binary.BigEndian.PutUint32(rootHeader[4:], uint32(8+body.Len()))
	var out bytes.Buffer
	_, _ = out.Write(rootHeader[:])
	_, _ = out.Write(body.Bytes())
	return out.Bytes(), nil
}

func main() {
	outPath := flag.String("out", "AppIcon.icns", "output .icns path")
	flag.Parse()
	icnsBytes, err := buildICNS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "makeicon failed: %v\n", err)
		os.Exit(1)
	} else if writeErr := os.WriteFile(*outPath, icnsBytes, 0644); writeErr != nil {
		fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", *outPath, writeErr)
		os.Exit(1)
	} else {
		// Icon file written cleanly.
	}
}
