package office

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CellStyle is the subset of a cellXfs entry used for value exchange.
type CellStyle struct {
	Date       bool
	Horizontal string
}

var builtinDateFormats = map[int]bool{14: true, 15: true, 16: true, 17: true, 18: true, 19: true, 20: true, 21: true, 22: true, 45: true, 46: true, 47: true}

var (
	fmtQuotedRe = regexp.MustCompile(`"[^"]*"|\\.|\[[^\]]*\]`)
	dateCodeRe  = regexp.MustCompile(`[dmyhs]`)
)

// Styles reads cellXfs number-format date flags and horizontal alignment.
func (wb *Workbook) Styles() []CellStyle {
	doc, err := wb.Pkg.XML("xl/styles.xml")
	if err != nil {
		return nil
	}
	custom := map[int]string{}
	for _, f := range All(Child(doc.Root(), NSS, "numFmts"), NSS, "numFmt") {
		id, _ := strconv.Atoi(f.SelectAttrValue("numFmtId", ""))
		custom[id] = f.SelectAttrValue("formatCode", "")
	}
	var out []CellStyle
	for _, xf := range Children(Child(doc.Root(), NSS, "cellXfs"), NSS, "xf") {
		id, _ := strconv.Atoi(xf.SelectAttrValue("numFmtId", "0"))
		date := builtinDateFormats[id]
		if code, ok := custom[id]; ok {
			stripped := strings.ToLower(fmtQuotedRe.ReplaceAllString(code, ""))
			date = dateCodeRe.MatchString(stripped)
		}
		out = append(out, CellStyle{Date: date, Horizontal: AttrOr(Child(xf, NSS, "alignment"), "horizontal", "")})
	}
	return out
}

// StyleOf returns the CellStyle of a cell element.
func StyleOf(styles []CellStyle, c interface{ SelectAttrValue(string, string) string }) CellStyle {
	i, err := strconv.Atoi(c.SelectAttrValue("s", "0"))
	if err != nil || i < 0 || i >= len(styles) {
		return CellStyle{}
	}
	return styles[i]
}

// SerialToTime converts an Excel serial date to time (1900 or 1904 epoch).
func SerialToTime(serial float64, date1904 bool) time.Time {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	} else if serial < 60 {
		base = base.AddDate(0, 0, 1)
	}
	days := math.Floor(serial)
	secs := math.Round((serial - days) * 86400)
	return base.AddDate(0, 0, int(days)).Add(time.Duration(secs) * time.Second)
}

// ISODate formats like Reference's datetime.isoformat().
func ISODate(t time.Time) string { return t.Format("2006-01-02T15:04:05") }
