package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"strconv"
)

type ChartLabelOverride struct {
	Separator      *string `json:"separator,omitempty"`
	ShowValue      *bool   `json:"show_value,omitempty"`
	ShowCategory   *bool   `json:"show_category,omitempty"`
	ShowSeries     *bool   `json:"show_series,omitempty"`
	ShowPercent    *bool   `json:"show_percent,omitempty"`
	ShowLegendKey  *bool   `json:"show_legend_key,omitempty"`
	ShowBubbleSize *bool   `json:"show_bubble_size,omitempty"`

	Index     int             `json:"index"`
	Text      *string         `json:"text,omitempty"`
	TextStyle *ChartTextStyle `json:"text_style,omitempty"`
	Fill      *DrawingStyle   `json:"fill,omitempty"`
	Outline   *TextOutline    `json:"outline,omitempty"`
	Position  string          `json:"position,omitempty"`
}

const labelOrder = "idx delete layout tx numFmt spPr txPr dLblPos showLegendKey showVal showCatName showSerName showPercent showBubbleSize separator extLst"
const labelsOrder = "dLbl delete numFmt spPr txPr dLblPos showLegendKey showVal showCatName showSerName showPercent showBubbleSize separator showLeaderLines leaderLines extLst"

func formatLabelOverrides(ser *etree.Element, overrides []ChartLabelOverride) error {
	if len(overrides) == 0 {
		return nil
	}
	switch ser.Parent().Tag {
	case "barChart", "bar3DChart", "lineChart", "line3DChart", "pieChart", "pie3DChart", "doughnutChart", "ofPieChart", "areaChart", "area3DChart", "scatterChart", "bubbleChart", "radarChart":
	default:
		return fmt.Errorf("label overrides unsupported for %s", ser.Parent().Tag)
	}
	values := office.Child(ser, cns, "val")
	if values == nil {
		values = office.Child(ser, cns, "yVal")
	}
	counts := office.All(values, cns, "ptCount")
	if len(counts) != 1 {
		return fmt.Errorf("label overrides require a value cache")
	}
	count, err := strconv.Atoi(counts[0].SelectAttrValue("val", ""))
	if err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, v := range overrides {
		if v.Index < 0 || v.Index >= count || seen[v.Index] {
			return fmt.Errorf("invalid or duplicate label index")
		}
		if v.ShowPercent != nil && *v.ShowPercent {
			switch ser.Parent().Tag {
			case "pieChart", "pie3DChart", "doughnutChart", "ofPieChart":
			default:
				return fmt.Errorf("percent labels require pie or doughnut")
			}
		}
		seen[v.Index] = true
		labels := chartEnsure(ser, "dLbls", seriesOrder)
		var label *etree.Element
		for _, n := range office.Children(labels, cns, "dLbl") {
			idx := office.Child(n, cns, "idx")
			if idx != nil && idx.SelectAttrValue("val", "") == strconv.Itoa(v.Index) {
				if label != nil {
					return fmt.Errorf("ambiguous label index")
				}
				label = n
			}
		}
		if label == nil {
			label = newChartChild(labels, "dLbl", labelsOrder)
			label.CreateElement("c:idx").CreateAttr("val", strconv.Itoa(v.Index))
		}
		if v.Separator != nil {
			chartEnsure(label, "separator", labelOrder).SetText(*v.Separator)
		}
		for _, flag := range []struct {
			tag   string
			value *bool
		}{
			{"showVal", v.ShowValue}, {"showCatName", v.ShowCategory}, {"showSerName", v.ShowSeries},
			{"showPercent", v.ShowPercent}, {"showLegendKey", v.ShowLegendKey}, {"showBubbleSize", v.ShowBubbleSize},
		} {
			if flag.value != nil {
				chartBool(label, flag.tag, *flag.value, labelOrder)
			}
		}
		if v.Position != "" {
			switch v.Position {
			case "bestFit", "b", "ctr", "inBase", "inEnd", "l", "outEnd", "r", "t":
			default:
				return fmt.Errorf("invalid label position")
			}
			chartEnsure(label, "dLblPos", labelOrder).CreateAttr("val", v.Position)
		}
		if v.Text != nil {
			tx := chartEnsure(label, "tx", labelOrder)
			for _, n := range tx.ChildElements() {
				tx.RemoveChild(n)
			}
			rich := tx.CreateElement("c:rich")
			rich.CreateAttr("xmlns:c", cns)
			rich.CreateAttr("xmlns:a", ans)
			rich.CreateElement("a:bodyPr")
			rich.CreateElement("a:lstStyle")
			rich.CreateElement("a:p").CreateElement("a:r").CreateElement("a:t").SetText(*v.Text)
		}
		if err := chartAppearance(label, v.Fill, v.Outline, v.TextStyle, labelOrder); err != nil {
			return err
		}
		if v.TextStyle != nil {
			tx := office.Child(label, cns, "tx")
			rich := office.Child(tx, cns, "rich")
			if rich != nil {
				// Rich-text overrides carry their own run properties, which win over txPr.
				if err := formatChartTextBody(rich, v.TextStyle); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
