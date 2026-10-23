package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func applyStructuredRunProperties(pkg *opc.Package, part string, body *etree.Element, paragraphs []Paragraph) error {
	ps := office.Children(body, ans, "p")
	for pi, para := range paragraphs {
		for ri, run := range para.Runs {
			if run.Hyperlink == "" && run.TextPaint == nil {
				continue
			}
			if pi >= len(ps) {
				return fmt.Errorf("structured run paragraph not found")
			}
			runs := office.Children(ps[pi], ans, "r")
			if ri >= len(runs) {
				return fmt.Errorf("structured run not found")
			}
			pr := office.Child(runs[ri], ans, "rPr")
			if pr == nil {
				pr = etree.NewElement("a:rPr")
				runs[ri].InsertChildAt(0, pr)
			}
			ensureDrawingPrefix(pr, runs[ri])
			if run.TextPaint != nil {
				if err := paintTextProperties(pr, run.TextPaint); err != nil {
					return err
				}
			}
			if run.Hyperlink == "" {
				continue
			}
			if err := setHyperlink(pkg, part, pr, run.Hyperlink); err != nil {
				return err
			}
			for _, h := range office.Children(pr, ans, "hlinkClick") {
				h.CreateAttr("xmlns:r", rns)
			}
		}
	}
	return nil
}
