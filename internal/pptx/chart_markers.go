package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"strconv"
)

type ChartMarkerFormat struct {
	Symbol  string        `json:"symbol,omitempty"`
	Size    *int          `json:"size,omitempty"`
	Fill    *DrawingStyle `json:"fill,omitempty"`
	Outline *TextOutline  `json:"outline,omitempty"`
}

func formatChartMarker(ser *etree.Element, m *ChartMarkerFormat) error {
	switch ser.Parent().Tag {
	case "lineChart", "line3DChart", "scatterChart", "radarChart":
	default:
		return fmt.Errorf("markers require line, scatter or radar charts")
	}
	if m.Symbol != "" {
		switch m.Symbol {
		case "auto", "circle", "diamond", "dot", "none", "plus", "square", "star", "triangle", "x":
		default:
			return fmt.Errorf("invalid marker symbol")
		}
	}
	if m.Size != nil && (*m.Size < 2 || *m.Size > 72) {
		return fmt.Errorf("marker size must be 2..72 points")
	}
	marker := chartEnsure(ser, "marker", seriesOrder)
	if m.Symbol != "" {
		chartEnsure(marker, "symbol", "symbol size spPr extLst").CreateAttr("val", m.Symbol)
	}
	if m.Size != nil {
		chartEnsure(marker, "size", "symbol size spPr extLst").CreateAttr("val", strconv.Itoa(*m.Size))
	}
	return chartAppearance(marker, m.Fill, m.Outline, nil, "symbol size spPr extLst")
}
