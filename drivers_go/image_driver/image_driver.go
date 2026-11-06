// drivers_go/image_driver/image_driver.go
package imagedriver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"

	"datahumper/drivers_go/driverlib"
)

// image_driver previews an image: it decodes just the header and lists the
// format, width, and height as leaves. PNG, JPEG, and GIF are decoded by the
// standard image registry; BMP dimensions are read straight from the BMP
// header because the standard library has no BMP decoder. Bytes that are not
// a recognizable image produce a single error resource. It is a leaf driver
// and never touches the network; it only parses bytes handed down by its
// parent.

func Run() {
	driverlib.Run("Image driver", listImage)
}

func listImage(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(raw) == 0:
		return nil, map[string][]byte{}, nil
	default:
	}

	format, width, height, err := imageHeader(raw)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}

	var resources []driverlib.Resource
	resources = driverlib.AppendKV(resources, "format", format)
	resources = driverlib.AppendKV(resources, "width", strconv.Itoa(width))
	resources = driverlib.AppendKV(resources, "height", strconv.Itoa(height))
	return resources, map[string][]byte{}, nil
}

// imageHeader identifies the image format and pixel dimensions from the
// header bytes alone, without decoding pixel data.
func imageHeader(raw []byte) (string, int, int, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	switch {
	case err == nil:
		return format, config.Width, config.Height, nil
	case isBMP(raw):
		width, height := bmpDimensions(raw)
		return "bmp", width, height, nil
	default:
		return "", 0, 0, fmt.Errorf("cannot decode the image: expected a PNG, JPEG, GIF, or BMP header, got %d bytes with error: %v", len(raw), err)
	}
}

// isBMP recognizes the BITMAPFILEHEADER magic plus enough bytes for the
// BITMAPINFOHEADER dimension fields read by bmpDimensions.
func isBMP(raw []byte) bool {
	switch {
	case len(raw) >= 26 && raw[0] == 'B' && raw[1] == 'M':
		return true
	default:
		return false
	}
}

// bmpDimensions reads the little-endian 32-bit width and height at offsets 18
// and 22 of the BMP header. A negative height means a top-down bitmap, so the
// magnitude is reported.
func bmpDimensions(raw []byte) (int, int) {
	width := int(int32(binary.LittleEndian.Uint32(raw[18:22])))
	height := int(int32(binary.LittleEndian.Uint32(raw[22:26])))
	switch {
	case height < 0:
		height = -height
	default:
	}
	return width, height
}
