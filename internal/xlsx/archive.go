package xlsx

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
)

// spreadsheetArchive follows openpyxl's new-workbook write order. Unrecognized
// parts are retained; exact archive equivalence is not asserted for those extensions.
func spreadsheetArchive(p *opc.Package, created time.Time, clock func() time.Time) ([]byte, error) {
	stamp := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }
	p.Set("docProps/app.xml", []byte(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Microsoft Excel Compatible / Openpyxl 3.1.5</Application><AppVersion>3.1</AppVersion></Properties>`))
	p.Set("docProps/core.xml", []byte(fmt.Sprintf(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"><dc:creator xmlns:dc="http://purl.org/dc/elements/1.1/">openpyxl</dc:creator><dcterms:created xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="dcterms:W3CDTF">%s</dcterms:created><dcterms:modified xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="dcterms:W3CDTF">%s</dcterms:modified></cp:coreProperties>`, stamp(created), stamp(clock()))))
	ordered := &opc.Package{}
	seen := map[string]bool{}
	add := func(n string) {
		if p.Has(n) && !seen[n] {
			for _, part := range p.Parts {
				if part.Name == n {
					ordered.Parts = append(ordered.Parts, part)
					break
				}
			}
			seen[n] = true
		}
	}
	for _, n := range []string{"docProps/app.xml", "docProps/core.xml", "xl/theme/theme1.xml"} {
		add(n)
	}
	for _, n := range p.Names() {
		if !strings.HasPrefix(n, "xl/worksheets/sheet") || !strings.HasSuffix(n, ".xml") {
			continue
		}
		add(n)
		rel := path.Join(path.Dir(n), "_rels", path.Base(n)+".rels")
		if p.Has(rel) {
			d, e := p.XML(rel)
			if e != nil {
				return nil, e
			}
			for _, r := range d.Root().ChildElements() {
				if strings.HasSuffix(r.SelectAttrValue("Type", ""), "/drawing") {
					target := r.SelectAttrValue("Target", "")
					if strings.HasPrefix(target, "/") {
						target = strings.TrimPrefix(target, "/")
					} else {
						target = path.Join(path.Dir(n), target)
					}
					add(target)
					add(path.Join(path.Dir(target), "_rels", path.Base(target)+".rels"))
				}
			}
		}
		add(rel)
	}
	for _, prefix := range []string{"xl/media/", "xl/charts/"} {
		for _, n := range p.Names() {
			if strings.HasPrefix(n, prefix) {
				add(n)
			}
		}
	}
	for _, n := range []string{"xl/styles.xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels"} {
		add(n)
	}
	for _, n := range p.Names() {
		if n != "[Content_Types].xml" {
			add(n)
		}
	}
	add("[Content_Types].xml")
	return opc.ReferenceZIPBytesWithAttrs(ordered, clock, zlibcompat.DeflateZIPLevel6, func(n string) uint32 {
		if strings.HasPrefix(n, "xl/worksheets/sheet") && strings.HasSuffix(n, ".xml") {
			return 0100600 << 16
		}
		return 0600 << 16
	})
}
