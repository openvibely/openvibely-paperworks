package pdf

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// reportDocument follows pinned ReportLab PDFDocument/PDFDictionary and
// SimpleDocTemplate defaults for one standalone fixed-layout page. Mixed pages,
// forms and flow documents continue to use the general writer. Producer/header
// strings are compatibility-profile metadata, not the implementation language.
func reportDocument(ctx context.Context, c *model.Context, pages []CreatePage) ([]byte, bool, error) {
	if len(pages) != 1 {
		return nil, false, nil
	}
	p := pages[0]
	if len(p.FixedParagraphs) == 0 || len(p.Text)+len(p.Tables)+len(p.Images)+len(p.TextFields)+len(p.Checkboxes)+len(p.RadioGroups)+len(p.ComboBoxes) > 0 {
		return nil, false, nil
	}
	pd, _, err := pageDict(ctx, c, 1)
	if err != nil {
		return nil, true, err
	}
	resources := dictOf(c.XRefTable, pd["Resources"]).d
	fonts := dictOf(c.XRefTable, resources["Font"]).d
	names := sortedDictKeys(fonts)
	// Font keys are allocated as F1, F2, ... in first-use order.
	sort.Slice(names, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimPrefix(names[i], "F"))
		b, _ := strconv.Atoi(strings.TrimPrefix(names[j], "F"))
		return a < b
	})
	objs := []types.Object{nil}
	fd := types.Dict{}
	for _, name := range names {
		fd[name] = *types.NewIndirectRef(len(objs)+1, 0)
		objs = append(objs, dictOf(c.XRefTable, fonts[name]).d)
	}
	objs[0] = fd
	pageID := len(objs) + 1
	catalogID, infoID, treeID, streamID := pageID+1, pageID+2, pageID+3, pageID+4
	page := pd.Clone().(types.Dict)
	page["Parent"] = *types.NewIndirectRef(treeID, 0)
	page["Contents"] = *types.NewIndirectRef(streamID, 0)
	page["Resources"] = types.Dict{"Font": *types.NewIndirectRef(1, 0), "ProcSet": resources["ProcSet"]}
	now := time.Now()
	stamp := strconv.FormatFloat(float64(now.UnixNano())/1e9, 'f', -1, 64)
	if !strings.Contains(stamp, ".") {
		stamp += ".0"
	}
	if epoch := strings.TrimSpace(os.Getenv("SOURCE_DATE_EPOCH")); epoch != "" {
		sec, e := strconv.ParseInt(epoch, 10, 64)
		if e != nil {
			return nil, true, fmt.Errorf("SOURCE_DATE_EPOCH: %w", e)
		}
		now = time.Unix(sec, 0).UTC()
		stamp = strconv.FormatInt(sec, 10)
	}
	_, zone := now.Zone()
	// Pinned TimeStamp uses whole offset hours and (offset % 3600) % 60.
	date := fmt.Sprintf("D:%s%+03d'%02d'", now.Format("20060102150405"), zone/3600, ((zone%3600)+3600)%60)
	str := func(s string) types.StringLiteral { return types.StringLiteral(s) }
	info := types.Dict{"Author": str("(anonymous)"), "CreationDate": str(date), "Creator": str("(unspecified)"), "Keywords": str(""), "ModDate": str(date), "Producer": str("ReportLab PDF Library - (opensource)"), "Subject": str("(unspecified)"), "Title": str("(anonymous)"), "Trapped": types.Name("False")}
	sd, _ := streamOf(c.XRefTable, pd["Contents"])
	if sd == nil {
		return nil, true, fmt.Errorf("missing fixed page stream")
	}
	objs = append(objs, page, types.Dict{"PageMode": types.Name("UseNone"), "Pages": *types.NewIndirectRef(treeID, 0), "Type": types.Name("Catalog")}, info, types.Dict{"Count": types.Integer(1), "Kids": types.Array{*types.NewIndirectRef(pageID, 0)}, "Type": types.Name("Pages")}, *sd)
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\x93\x8c\x8b\x9e ReportLab Generated PDF document (opensource)\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		v, e := reportObject(o)
		if e != nil {
			return nil, true, e
		}
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, v)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	id := md5.Sum([]byte("a reportlab document" + stamp + "(anonymous)" + "(anonymous)" + "(unspecified)"))
	fmt.Fprintf(&b, "trailer\n<<\n/ID \n[<%x><%x>]\n%% ReportLab generated PDF document -- digest (opensource)\n\n/Info %d 0 R\n/Root %d 0 R\n/Size %d\n>>\nstartxref\n%d\n%%%%EOF\n", id, id, infoID, catalogID, len(objs)+1, xref)
	return b.Bytes(), true, nil
}

func reportObject(o types.Object) (string, error) {
	switch v := o.(type) {
	case types.Dict:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for i, k := range keys {
			if i > 0 && i%6 == 0 {
				parts = append(parts, "\n ")
			}
			s, e := reportObject(v[k])
			if e != nil {
				return "", e
			}
			parts = append(parts, "/"+k+" "+s)
		}
		return "<<\n" + strings.Join(parts, " ") + "\n>>", nil
	case types.Array:
		parts := []string{}
		for i, a := range v {
			if i > 0 && i%10 == 0 {
				parts = append(parts, "\n ")
			}
			s, e := reportObject(a)
			if e != nil {
				return "", e
			}
			parts = append(parts, s)
		}
		return "[ " + strings.Join(parts, " ") + " ]", nil
	case types.Name:
		return "/" + string(v), nil
	case types.StringLiteral:
		return "(" + fixedString(string(v)) + ")", nil
	case types.Integer:
		return strconv.Itoa(int(v)), nil
	case types.Float:
		return reportNumber(float64(v)), nil
	case types.IndirectRef:
		return fmt.Sprintf("%d %d R", v.ObjectNumber, v.GenerationNumber), nil
	case types.StreamDict:
		d, e := reportObject(v.Dict)
		return d + "\nstream\n" + string(v.Raw) + "endstream", e
	default:
		return "", fmt.Errorf("unsupported report object %T", o)
	}
}
