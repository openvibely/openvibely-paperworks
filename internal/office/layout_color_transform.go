package office

import (
	"fmt"
	"math"
	"strconv"

	"github.com/beevik/etree"
)

func layoutTransformColor(hex string, transforms []*etree.Element) (string, float64, bool) {
	if len(hex) != 6 {
		return "", 0, false
	}
	var rgb [3]float64
	for i := range rgb {
		n, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return "", 0, false
		}
		rgb[i] = float64(n) / 255
	}
	rgb, alpha, ok := drawingMLColorTransforms(rgb, transforms)
	if !ok {
		return "", 0, false
	}
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(rgb[0]*255)), int(math.Round(rgb[1]*255)), int(math.Round(rgb[2]*255))), alpha, true
}
