package wordedit

import (
	"github.com/beevik/etree"
	"strconv"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// These checks supplement (not replace) the original merge method, which only
// required numbering-part presence. The pinned composer restarts ListNumber and
// imports no unused numbering definitions from this appendix.
func TestMergeRestartsAppendedListsAndRegistersImages(t *testing.T) {
	for _, copies := range []int{1, 2} {
		t.Run(strconv.Itoa(copies), func(t *testing.T) {
			appendix := referenceOriginalWordFixture(t, "merge-append")
			files := []any{}
			for i := 0; i < copies; i++ {
				files = append(files, appendix)
			}
			out := mustEdit(t, base(t), map[string]any{"operation": "merge", "files": files})
			numbers := referenceXML(t, out, "word/numbering.xml")
			referenceEqual(t, len(office.Children(numbers, nsW, "abstractNum")), 9)
			nums := office.Children(numbers, nsW, "num")
			referenceEqual(t, len(nums), 9+copies)
			doc := referenceXML(t, out, "word/document.xml")
			found := 0
			for _, p := range office.All(doc, nsW, "p") {
				if referenceParagraphText(p) != "First item" {
					continue
				}
				found++
				props := office.Child(p, nsW, "pPr")
				n := office.Child(office.Child(props, nsW, "numPr"), nsW, "numId")
				want := strconv.Itoa(9 + found)
				referenceEqual(t, getW(n, "val"), want)
				var definitionFound bool
				for _, n := range nums {
					if getW(n, "numId") != want {
						continue
					}
					definitionFound = true
					referenceEqual(t, getW(office.Child(n, nsW, "abstractNumId"), "val"), "7")
					override := office.Child(n, nsW, "lvlOverride")
					referenceEqual(t, getW(override, "ilvl"), "0")
					referenceEqual(t, getW(office.Child(override, nsW, "startOverride"), "val"), "1")
				}
				if !definitionFound {
					t.Fatal("missing restarted numbering definition")
				}
			}
			referenceEqual(t, found, copies)
			for i, n := range office.All(doc, "http://schemas.openxmlformats.org/drawingml/2006/picture", "cNvPr") {
				referenceEqual(t, n.SelectAttrValue("id", ""), strconv.Itoa(i+1))
			}
			ct := referenceXML(t, out, "[Content_Types].xml")
			defaults := office.Children(ct, opc.NSCT, "Default")
			png := 0
			for _, n := range defaults {
				if n.SelectAttrValue("Extension", "") == "png" {
					png++
					referenceEqual(t, n.SelectAttrValue("ContentType", ""), "image/png")
				}
			}
			referenceEqual(t, png, 1)
			if copies == 2 {
				once := mustEdit(t, base(t), map[string]any{"operation": "merge", "files": []any{appendix}})
				referenceEqual(t, countImages(t, out), countImages(t, once))
				if countImages(t, once) == 0 {
					t.Fatal("merge fixture has no images to deduplicate")
				}
			}
		})
	}
}

func countImages(t *testing.T, file string) int {
	t.Helper()
	p, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, name := range p.Names() {
		if strings.HasPrefix(p.ContentType(name), "image/") {
			n++
		}
	}
	return n
}

func TestMergeRemapsExplicitNumberingDefinition(t *testing.T) {
	src := referenceOriginalWordFixture(t, "merge-append")
	src = referenceRewrite(t, src, func(p *opc.Package) {
		referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
			for _, p := range office.All(root, nsW, "p") {
				if referenceParagraphText(p) != "First item" {
					continue
				}
				props := office.Child(p, nsW, "pPr")
				props.RemoveChild(office.Child(props, nsW, "pStyle"))
				setW(props.CreateElement("w:numPr").CreateElement("w:numId"), "val", "5")
			}
		})
	})
	out := mustEdit(t, base(t), map[string]any{"operation": "merge", "files": []any{src}})
	root := referenceXML(t, out, "word/numbering.xml")
	referenceEqual(t, len(office.Children(root, nsW, "abstractNum")), 10)
	referenceEqual(t, len(office.Children(root, nsW, "num")), 10)
	var abs string
	for _, n := range office.Children(root, nsW, "num") {
		if getW(n, "numId") == "10" {
			abs = getW(office.Child(n, nsW, "abstractNumId"), "val")
		}
	}
	referenceEqual(t, abs, "9")
}

func mergeBookmarkBody(name, text, field string) string {
	return `<w:p><w:bookmarkStart w:id="0" w:name="` + name + `"/><w:r><w:t>` + text + `</w:t></w:r><w:bookmarkEnd w:id="0"/></w:p><w:p>` + field + `</w:p><w:p><w:hyperlink w:anchor="` + name + `"><w:r><w:t>jump</w:t></w:r></w:hyperlink></w:p>`
}

func TestMergeBookmarkReferences(t *testing.T) {
	simple := `<w:fldSimple w:instr=" REF Summary \h "><w:r><w:t>stale</w:t></w:r></w:fldSimple>`
	complex := `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> RE</w:instrText></w:r><w:r><w:instrText>F Sum</w:instrText></w:r><w:r><w:instrText xml:space="preserve">mary \h </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>stale</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`
	for _, field := range []string{simple, complex} {
		t.Run(strconv.Itoa(len(field)), func(t *testing.T) {
			dst := mk(t, "destination", mergeBookmarkBody("Summary", "Original summary", field), nil)
			// Reserve a name that allocation would otherwise choose, even though its
			// bookmark occurs after the conflicting bookmark in the source.
			src := mk(t, "source", mergeBookmarkBody("Summary", "Appendix summary", field)+`<w:p><w:bookmarkStart w:id="1" w:name="PaperworksMerge1"/><w:r><w:t>suffix</w:t></w:r><w:bookmarkEnd w:id="1"/></w:p>`, nil)
			out := mustEdit(t, dst, map[string]any{"operation": "merge", "files": []any{src, src}})
			root := referenceXML(t, out, "word/document.xml")
			starts := all(root, "bookmarkStart")
			ends := all(root, "bookmarkEnd")
			referenceEqual(t, len(starts), 5)
			referenceEqual(t, len(ends), 5)
			names, ids := map[string]bool{}, map[string]bool{}
			for i, b := range starts {
				name, id := getW(b, "name"), getW(b, "id")
				if names[name] || ids[id] {
					t.Fatal("duplicate bookmark name or ID", name, id)
				}
				if err := bookmarkName(name); err != nil {
					t.Fatal(err)
				}
				names[name], ids[id] = true, true
				referenceEqual(t, getW(ends[i], "id"), id)
			}
			referenceEqual(t, getW(starts[0], "name"), "Summary")
			referenceEqual(t, getW(starts[2], "name"), "PaperworksMerge1")
			links := all(root, "hyperlink")
			for i, index := range []int{0, 1, 3} {
				referenceEqual(t, getW(links[i], "anchor"), getW(starts[index], "name"))
			}
			materialized := mustEdit(t, out, map[string]any{"operation": "materialize", "only": []any{"REF"}})
			saved := referenceXML(t, materialized, "word/document.xml")
			paragraphs := all(saved, "p")
			referenceEqual(t, office.Text(paragraphs[1], nsW, "t"), "Original summary")
			referenceEqual(t, office.Text(paragraphs[4], nsW, "t"), "Appendix summary")
			referenceEqual(t, office.Text(paragraphs[8], nsW, "t"), "Appendix summary")
		})
	}
}

func TestMergeBookmarkInstructionForms(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		reject   bool
	}{
		{` REF Summary \h `, ` REF NewName \h `, false},
		{` pageref "Summary" \h \p `, ` pageref "NewName" \h \p `, false},
		{` NOTEREF Summary \f `, ` NOTEREF NewName \f `, false},
		{` HYPERLINK \l "Summary" \o "Summary tooltip" `, ` HYPERLINK \l "NewName" \o "Summary tooltip" `, false},
		{` HYPERLINK "https://example.com" \l "Summary" `, ` HYPERLINK "https://example.com" \l "Summary" `, false},
		{` REF Unique \h `, ` REF Unique \h `, false},
		{` TOC \b Summary `, "", true},
		{` =Summary + 1 `, "", true},
		{` REF \h Summary `, "", true},
		{` HYPERLINK \o "tooltip" \l Summary `, "", true},
		{` REF "Summary `, "", true},
		{` SET Other Summary `, "", true},
		{` SET Other `, "", true},
		{` SET "" "Summary" `, "", true},
		{` ASK "" "Summary" `, "", true},
		{` ASK \d "Summary" `, "", true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := rewriteMergedInstruction(tc.in, map[string]string{"Summary": "NewName"})
			if tc.reject {
				if err == nil {
					t.Fatal("expected explicit rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			referenceEqual(t, got, tc.want)
		})
	}
}

func TestMergeBookmarkNoConflictAndExternalLink(t *testing.T) {
	dst := mk(t, "destination", mergeBookmarkBody("Original", "Original summary", `<w:fldSimple w:instr="REF Original"><w:r><w:t>stale</w:t></w:r></w:fldSimple>`), nil)
	src := mk(t, "source", mergeBookmarkBody("Summary", "Appendix summary", `<w:fldSimple w:instr="REF Summary"><w:r><w:t>stale</w:t></w:r></w:fldSimple>`)+`<w:p><w:hyperlink r:id="external" w:anchor="Summary"><w:r><w:t>external</w:t></w:r></w:hyperlink></w:p>`, nil)
	src = referenceRewrite(t, src, func(p *opc.Package) {
		if err := addRelWithMode(p, "word/document.xml", "external", nsR+"/hyperlink", "https://example.com", "External"); err != nil {
			t.Fatal(err)
		}
	})
	// The second append exercises the external anchor during a name collision.
	out := mustEdit(t, dst, map[string]any{"operation": "merge", "files": []any{src, src}})
	root := referenceXML(t, out, "word/document.xml")
	starts := all(root, "bookmarkStart")
	referenceEqual(t, getW(starts[1], "name"), "Summary")
	links := all(root, "hyperlink")
	referenceEqual(t, getW(links[2], "anchor"), "Summary")
	referenceEqual(t, getW(links[4], "anchor"), "Summary")
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := pkg.Rels("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []*etree.Element{links[2], links[4]} {
		found := false
		for _, rel := range rels {
			if rel.ID == office.AttrV(link, nsR, "id") {
				referenceEqual(t, rel.Target, "https://example.com")
				referenceEqual(t, rel.TargetMode, "External")
				found = true
			}
		}
		if !found {
			t.Fatal("lost external relationship")
		}
	}
}

func TestMergeRejectsUnsupportedBookmarkReferences(t *testing.T) {
	dst := mk(t, "destination", mergeBookmarkBody("Summary", "original", ""), nil)
	for _, field := range []string{
		`<w:fldSimple w:instr="TOC \b Summary"/>`,
		// A literal QUOTE wrapper must not hide an unsupported reference in
		// its nested result fields.
		`<w:fldSimple w:instr="QUOTE &quot;Summary&quot;"><w:fldSimple w:instr="TOC \b Summary"/></w:fldSimple>`,
		`<w:fldSimple w:instr="SET Other &quot;Summary&quot;"><w:fldSimple w:instr="TOC \b Summary"/></w:fldSimple>`,
		strings.Replace(complexField(" SET Other ", "cached"), `<w:r><w:fldChar w:fldCharType="separate"/></w:r>`, complexField(" REF Summary ", "cached")+`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`, 1),
		`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText>REF </w:instrText></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r>`,
	} {
		src := mk(t, "source", mergeBookmarkBody("Summary", "appendix", field), nil)
		wantErr(t, dst, map[string]any{"operation": "merge", "files": []any{src}}, "unsupported.*(bookmark|nested)")
	}
}

func TestMergeBookmarkPageReferenceAcrossParagraphs(t *testing.T) {
	dst := mk(t, "destination", mergeBookmarkBody("Summary", "original", ""), nil)
	src := mk(t, "source", mergeBookmarkBody("Summary", "appendix", `<w:fldSimple w:instr=" PAGEREF &quot;Summary&quot; \h \p "><w:r><w:t>1</w:t></w:r></w:fldSimple>`)+`<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE</w:instrText></w:r></w:p><w:p><w:r><w:instrText>REF &quot;Sum</w:instrText></w:r><w:r><w:instrText xml:space="preserve">mary&quot; \h \p </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>1</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`, nil)
	out := mustEdit(t, dst, map[string]any{"operation": "merge", "files": []any{src}})
	root := referenceXML(t, out, "word/document.xml")
	name := getW(all(root, "bookmarkStart")[1], "name")
	want := ` PAGEREF "` + name + `" \h \p `
	referenceEqual(t, getW(all(root, "fldSimple")[0], "instr"), want)
	referenceEqual(t, office.Text(root, nsW, "instrText"), want)
}

func TestMergeReservesDestinationStoryBookmarks(t *testing.T) {
	for _, story := range []struct{ part, tag string }{
		{"word/header1.xml", "hdr"},
		{"word/footer1.xml", "ftr"},
	} {
		t.Run(story.tag, func(t *testing.T) {
			field := simpleField(` REF Summary \h `, "stale")
			content := mergeBookmarkBody("Summary", "Original summary", field) + `<w:p><w:bookmarkStart w:id="1" w:name="PaperworksMerge1"/><w:r><w:t>reserved</w:t></w:r><w:bookmarkEnd w:id="1"/></w:p>`
			dst := mk(t, "destination", `<w:p>`+field+`</w:p>`, map[string]string{
				story.part: `<w:` + story.tag + ` ` + wNS + `>` + content + `</w:` + story.tag + `>`,
			})
			dst = referenceRewrite(t, dst, func(p *opc.Package) {
				kind := "header"
				if story.tag == "ftr" {
					kind = "footer"
				}
				if err := p.EnsureContentType(story.part, ctBase+kind+"+xml"); err != nil {
					t.Fatal(err)
				}
				id, err := p.AddRel("word/document.xml", relBase+kind, strings.TrimPrefix(story.part, "word/"), false)
				if err != nil {
					t.Fatal(err)
				}
				referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
					ref := all(root, "sectPr")[0].CreateElement("w:" + kind + "Reference")
					setW(ref, "type", "default")
					ref.CreateAttr("r:id", id)
				})
			})
			src := mk(t, "source", mergeBookmarkBody("Summary", "Appendix summary", field), nil)
			out := mustEdit(t, dst, map[string]any{"operation": "merge", "files": []any{src, src}})
			root := referenceXML(t, out, "word/document.xml")
			starts := all(root, "bookmarkStart")
			referenceEqual(t, len(starts), 2)
			seen := map[string]bool{"Summary": true, "PaperworksMerge1": true}
			for _, start := range starts {
				name := getW(start, "name")
				if seen[name] {
					t.Fatal("unreserved name", name)
				}
				seen[name] = true
			}
			original := referenceXML(t, out, story.part)
			referenceEqual(t, getW(all(original, "bookmarkStart")[0], "name"), "Summary")
			referenceEqual(t, getW(all(original, "fldSimple")[0], "instr"), ` REF Summary \h `)
			out = mustEdit(t, out, map[string]any{"operation": "materialize", "only": []any{"REF"}})
			root = referenceXML(t, out, "word/document.xml")
			fields := all(root, "fldSimple")
			referenceEqual(t, office.Text(fields[0], nsW, "t"), "Original summary")
			referenceEqual(t, office.Text(fields[1], nsW, "t"), "Appendix summary")
			referenceEqual(t, office.Text(fields[2], nsW, "t"), "Appendix summary")
			original = referenceXML(t, out, story.part)
			referenceEqual(t, office.Text(all(original, "fldSimple")[0], nsW, "t"), "Original summary")
		})
	}
}

func TestMergePreservesUnrelatedFieldInstructions(t *testing.T) {
	for _, instr := range []string{
		` FILLIN "Summary" `,
		` fillin "Enter text" \d "Summary" \o `,
		` FILLIN "Summary prompt" \d "Summary default" \* MERGEFORMAT `,
		` ASK Other "Summary" `,
		` ask "Other" "Enter text" \d "Summary" \o `,
		` ASK Other "Summary prompt" \d "Summary default" \* MERGEFORMAT `,
		` QUOTE "Summary" `,
		` quote "Summary and other literal text" \* MERGEFORMAT `,
		` SET Other "Summary" `,
		` set "Other" "Summary and other literal text" \* MERGEFORMAT `,
		` SET Other 25 `,
		` SET Other 10% `,
		` DOCPROPERTY Summary `,
		` DOCPROPERTY "Summary" \* MERGEFORMAT `,
		` DOCVARIABLE Summary `,
		` MERGEFIELD Summary \b "Summary" `,
		` IF 1 = 1 "Summary" "Other" `,
		` IF 1=1 "Summary" "Other" `,
		` IF 1 = 2 "Other" "Summary" `,
		` IF "Summary" = "Summary" "Yes" "No" `,
		` IF 1 = 1 Summary Other `,
		` SEQ Summary \* ARABIC `,
		` STYLEREF Summary `,
		` TOC \t "Summary,1" `,
	} {
		for _, complex := range []bool{false, true} {
			t.Run(instr+strconv.FormatBool(complex), func(t *testing.T) {
				dst := mk(t, "destination", mergeBookmarkBody("Summary", "original", ""), nil)
				encoded := strings.ReplaceAll(instr, `"`, `&quot;`)
				field := simpleField(encoded, "cached")
				if complex {
					field = `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve">` + encoded[:4] + `</w:instrText></w:r><w:r><w:instrText xml:space="preserve">` + encoded[4:] + `</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>cached</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`
				}
				src := mk(t, "source", mergeBookmarkBody("Summary", "appendix", field), nil)
				out := mustEdit(t, dst, map[string]any{"operation": "merge", "files": []any{src}})
				root := referenceXML(t, out, "word/document.xml")
				starts := all(root, "bookmarkStart")
				referenceEqual(t, len(starts), 2)
				referenceEqual(t, getW(starts[0], "name"), "Summary")
				if getW(starts[1], "name") == "Summary" {
					t.Fatal("imported bookmark was not renamed")
				}
				referenceEqual(t, office.Text(all(root, "p")[4], nsW, "t"), "cached")
				if complex {
					referenceEqual(t, office.Text(root, nsW, "instrText"), instr)
				} else {
					referenceEqual(t, getW(all(root, "fldSimple")[0], "instr"), instr)
				}
			})
		}
	}
}

func TestMergeRejectsActualUnsupportedBookmarkOperands(t *testing.T) {
	dst := mk(t, "destination", mergeBookmarkBody("Summary", "original", ""), nil)
	for _, instr := range []string{
		` ASK Summary "Enter text" `,
		` ask "Summary" "Other prompt" \d "Other default" \o `,
		` ASK Summary "Summary prompt" \d "Summary default" `,
		` SET Summary "Other" `,
		` set "Summary" "Summary" `,
		` SET Summary 25 `,
		` TOC \b "Summary" `,
		` TOA \b Summary `,
		` INDEX \b "Summary" `,
		` SEQ Figure Summary `,
		` SEQ Figure "Summary" `,
		` IF Summary = 1 "Yes" "No" `,
		` IF Summary>=1 "Yes" "No" `,
		` IF 1 = Summary "Yes" "No" `,
		` =Summary + 1 `,
		` Summary `,
	} {
		for _, complex := range []bool{false, true} {
			t.Run(instr+strconv.FormatBool(complex), func(t *testing.T) {
				field := simpleField(strings.ReplaceAll(instr, `"`, `&quot;`), "cached")
				if complex {
					field = complexField(instr, "cached")
				}
				src := mk(t, "source", mergeBookmarkBody("Summary", "appendix", field), nil)
				wantErr(t, dst, map[string]any{"operation": "merge", "files": []any{src}}, "unsupported bookmark reference")
			})
		}
	}
}

func TestMergePromptFieldsDoNotHideNestedReferences(t *testing.T) {
	dst := mk(t, "destination", mergeBookmarkBody("Summary", "original", ""), nil)
	for _, instr := range []string{` FILLIN "Summary" \d "Summary" `, ` ASK Other "Summary" \d "Summary" `} {
		t.Run(instr, func(t *testing.T) {
			// Nested result fields must still be checked even when the surrounding
			// prompt and default contain only literal text.
			nestedResult := `<w:fldSimple w:instr="` + strings.ReplaceAll(instr, `"`, `&quot;`) + `"><w:fldSimple w:instr="TOC \b Summary"/></w:fldSimple>`
			// Dynamic instructions remain unsupported rather than silently letting
			// the nested bookmark reference resolve to the destination.
			separator := `<w:r><w:fldChar w:fldCharType="separate"/></w:r>`
			nestedInstruction := strings.Replace(complexField(instr, "cached"), separator, complexField(" REF Summary ", "cached")+separator, 1)
			for _, field := range []string{nestedResult, nestedInstruction} {
				src := mk(t, "source", mergeBookmarkBody("Summary", "appendix", field), nil)
				wantErr(t, dst, map[string]any{"operation": "merge", "files": []any{src}}, "unsupported.*(bookmark|nested)")
			}
		})
	}
}
