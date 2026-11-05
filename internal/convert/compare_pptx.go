package convert

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const officeDocumentRelationship = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
const slideRelationship = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide"

// PPTXTextChange describes a text-bearing shape change on a matched or unmatched
// slide. Slide and shape numbers are one-based when present.
type PPTXTextChange struct {
	Slide         int    `json:"slide"`
	BeforeSlide   int    `json:"before_slide,omitempty"`
	AfterSlide    int    `json:"after_slide,omitempty"`
	Status        string `json:"status"`
	ShapeID       string `json:"shape_id,omitempty"`
	BeforeShapeID string `json:"before_shape_id,omitempty"`
	AfterShapeID  string `json:"after_shape_id,omitempty"`
	Before        string `json:"before"`
	After         string `json:"after"`
}

type pptxTextShape struct {
	ID   string
	Name string
	Text string
}

type pptxTextSlide struct {
	ID        string
	Part      string
	Number    int
	Shapes    []pptxTextShape
	Signature string
}

type pptxSlidePair struct {
	before int
	after  int
}

type pptxShapePair struct {
	before int
	after  int
}

const pptxSlideMatchingDescription = "Slides pair first by a unique fingerprint of their combined text (which supports reordering), then by a unique persistent presentation slide ID. Remaining slides pair in presentation order only within corresponding gaps when the gap counts agree, known matches are not reordered, and at least one slide ID is missing in each fallback pair. Unmatched slides with different available IDs are reported as removed or added. Shapes pair by unique ID, then unique name or identical combined text when the candidates do not have different available IDs, then presentation order for unmatched shapes with a missing ID when the remaining gap counts agree. Text is concatenated in XML reading order, with paragraph and explicit line-break boundaries retained."

func pptxPresentationPart(pkg *opc.Package) (string, error) {
	rels, err := pkg.Rels("")
	if err != nil {
		return "", err
	}
	for _, rel := range rels {
		if rel.Type == officeDocumentRelationship && rel.TargetMode != "External" {
			part := opc.Resolve("", rel.Target)
			if pkg.Has(part) {
				return part, nil
			}
			return "", fmt.Errorf("presentation relationship target %q is missing", part)
		}
	}
	if pkg.Has("ppt/presentation.xml") {
		return "ppt/presentation.xml", nil
	}
	return "", fmt.Errorf("PPTX presentation part not found")
}

func pptxTextSlides(path string) ([]pptxTextSlide, error) {
	pkg, err := opc.Open(path)
	if err != nil {
		return nil, err
	}
	presentationPart, err := pptxPresentationPart(pkg)
	if err != nil {
		return nil, err
	}
	presentation, err := pkg.XML(presentationPart)
	if err != nil {
		return nil, err
	}
	presentationRels, err := pkg.Rels(presentationPart)
	if err != nil {
		return nil, err
	}
	relByID := make(map[string]opc.Relationship, len(presentationRels))
	for _, rel := range presentationRels {
		relByID[rel.ID] = rel
	}

	var ordered []struct{ id, part string }
	sldIDList := office.Child(presentation.Root(), office.NSP, "sldIdLst")
	for _, sldID := range office.Children(sldIDList, office.NSP, "sldId") {
		relID := office.AttrV(sldID, office.NSR, "id")
		rel, ok := relByID[relID]
		if !ok || rel.Type != slideRelationship || rel.TargetMode == "External" {
			return nil, fmt.Errorf("presentation slide relationship %q is missing or invalid", relID)
		}
		part := opc.Resolve(presentationPart, rel.Target)
		if !pkg.Has(part) {
			return nil, fmt.Errorf("presentation slide part %q is missing", part)
		}
		ordered = append(ordered, struct{ id, part string }{office.AttrV(sldID, "", "id"), part})
	}
	if sldIDList == nil {
		// Older or incomplete packages may omit the list. Use natural numbered
		// slide part order as a deterministic fallback, never ZIP member order.
		for _, name := range pkg.Names() {
			if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") && !strings.Contains(name, "/_rels/") {
				ordered = append(ordered, struct{ id, part string }{part: name})
			}
		}
		sort.Slice(ordered, func(i, j int) bool { return naturalSlidePartLess(ordered[i].part, ordered[j].part) })
	}

	slides := make([]pptxTextSlide, 0, len(ordered))
	for i, item := range ordered {
		doc, err := pkg.XML(item.part)
		if err != nil {
			return nil, err
		}
		slide := pptxTextSlide{ID: item.id, Part: item.part, Number: i + 1}
		spTree := office.Child(office.Child(doc.Root(), office.NSP, "cSld"), office.NSP, "spTree")
		collectPPTXTextShapes(spTree, &slide.Shapes)
		slide.Signature = pptxSlideTextSignature(slide.Shapes)
		slides = append(slides, slide)
	}
	return slides, nil
}

func naturalSlidePartLess(a, b string) bool {
	base := func(value string) string { return strings.TrimSuffix(filepath.Base(value), ".xml") }
	left, right := base(a), base(b)
	li, lj := len(left), len(right)
	for li > 0 && left[li-1] >= '0' && left[li-1] <= '9' {
		li--
	}
	for lj > 0 && right[lj-1] >= '0' && right[lj-1] <= '9' {
		lj--
	}
	if li == lj && left[:li] == right[:lj] {
		var ln, rn int
		_, _ = fmt.Sscanf(left[li:], "%d", &ln)
		_, _ = fmt.Sscanf(right[lj:], "%d", &rn)
		if ln != rn {
			return ln < rn
		}
	}
	return left < right
}

func collectPPTXTextShapes(root *etree.Element, shapes *[]pptxTextShape) {
	if root == nil {
		return
	}
	var walk func(*etree.Element)
	walk = func(node *etree.Element) {
		for _, child := range node.ChildElements() {
			if child.NamespaceURI() != office.NSP {
				continue
			}
			if child.Tag == "grpSp" {
				walk(child)
				continue
			}
			if child.Tag != "sp" && child.Tag != "graphicFrame" && child.Tag != "cxnSp" {
				continue
			}
			shape := pptxTextShape{}
			if props := office.All(child, office.NSP, "cNvPr"); len(props) > 0 {
				shape.ID = office.AttrV(props[0], "", "id")
				shape.Name = office.AttrV(props[0], "", "name")
			}
			shape.Text = pptxShapeText(child)
			hasTextBody := len(office.All(child, office.NSP, "txBody")) != 0 || len(office.All(child, office.NSA, "txBody")) != 0
			if shape.Text != "" || hasTextBody {
				*shapes = append(*shapes, shape)
			}
		}
	}
	walk(root)
}

func pptxShapeText(shape *etree.Element) string {
	paragraphs := office.All(shape, office.NSA, "p")
	var out strings.Builder
	for i, para := range paragraphs {
		if i > 0 {
			out.WriteByte('\n')
		}
		var walk func(*etree.Element)
		walk = func(node *etree.Element) {
			for _, child := range node.ChildElements() {
				if child.NamespaceURI() != office.NSA {
					continue
				}
				switch child.Tag {
				case "t":
					out.WriteString(child.Text())
				case "br":
					out.WriteByte('\n')
				case "tab":
					out.WriteByte('\t')
				default:
					walk(child)
				}
			}
		}
		walk(para)
	}
	return out.String()
}

func pptxSlideTextSignature(shapes []pptxTextShape) string {
	texts := make([]string, 0, len(shapes))
	for _, shape := range shapes {
		if shape.Text != "" {
			texts = append(texts, shape.Text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	sort.Strings(texts)
	var b strings.Builder
	for _, text := range texts {
		fmt.Fprintf(&b, "%d:%s;", len(text), text)
	}
	return b.String()
}

func matchPPTXSlides(before, after []pptxTextSlide) ([]pptxSlidePair, []int, []int) {
	beforeToAfter := make(map[int]int)
	afterUsed := make([]bool, len(after))
	pair := func(bi, ai int) {
		beforeToAfter[bi], afterUsed[ai] = ai, true
	}

	beforeByText, afterByText := make(map[string][]int), make(map[string][]int)
	for i, slide := range before {
		if slide.Signature != "" {
			beforeByText[slide.Signature] = append(beforeByText[slide.Signature], i)
		}
	}
	for i, slide := range after {
		if slide.Signature != "" {
			afterByText[slide.Signature] = append(afterByText[slide.Signature], i)
		}
	}
	for signature, bi := range beforeByText {
		ai := afterByText[signature]
		if len(bi) == 1 && len(ai) == 1 {
			pair(bi[0], ai[0])
		}
	}

	beforeIDs, afterIDs := make(map[string][]int), make(map[string][]int)
	for i, slide := range before {
		if _, ok := beforeToAfter[i]; !ok && slide.ID != "" {
			beforeIDs[slide.ID] = append(beforeIDs[slide.ID], i)
		}
	}
	for i, slide := range after {
		if !afterUsed[i] && slide.ID != "" {
			afterIDs[slide.ID] = append(afterIDs[slide.ID], i)
		}
	}
	for id, bi := range beforeIDs {
		ai := afterIDs[id]
		if len(bi) == 1 && len(ai) == 1 {
			pair(bi[0], ai[0])
		}
	}

	anchors := make([]pptxSlidePair, 0, len(beforeToAfter))
	for bi, ai := range beforeToAfter {
		anchors = append(anchors, pptxSlidePair{before: bi, after: ai})
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].before < anchors[j].before })
	monotonic := true
	for i := 1; i < len(anchors); i++ {
		if anchors[i-1].after >= anchors[i].after {
			monotonic = false
			break
		}
	}
	if monotonic {
		boundaries := append([]pptxSlidePair{{before: -1, after: -1}}, anchors...)
		boundaries = append(boundaries, pptxSlidePair{before: len(before), after: len(after)})
		for i := 1; i < len(boundaries); i++ {
			left, right := boundaries[i-1], boundaries[i]
			beforeGap := make([]int, 0)
			afterGap := make([]int, 0)
			for j := left.before + 1; j < right.before; j++ {
				if _, ok := beforeToAfter[j]; !ok {
					beforeGap = append(beforeGap, j)
				}
			}
			for j := left.after + 1; j < right.after; j++ {
				if !afterUsed[j] {
					afterGap = append(afterGap, j)
				}
			}
			if len(beforeGap) == len(afterGap) {
				for j := range beforeGap {
					bi, ai := beforeGap[j], afterGap[j]
					// Different available slide IDs identify separate slides. Positional
					// fallback is only safe when at least one side lacks an ID.
					if before[bi].ID == "" || after[ai].ID == "" {
						pair(bi, ai)
					}
				}
			}
		}
	}

	pairs := make([]pptxSlidePair, 0, len(beforeToAfter))
	removed := make([]int, 0)
	for bi := range before {
		if ai, ok := beforeToAfter[bi]; ok {
			pairs = append(pairs, pptxSlidePair{before: bi, after: ai})
		} else {
			removed = append(removed, bi)
		}
	}
	added := make([]int, 0)
	for ai, used := range afterUsed {
		if !used {
			added = append(added, ai)
		}
	}
	return pairs, removed, added
}

func matchPPTXShapes(before, after []pptxTextShape) ([]pptxShapePair, []int, []int) {
	beforeToAfter := make(map[int]int)
	afterUsed := make([]bool, len(after))
	pair := func(bi, ai int) { beforeToAfter[bi], afterUsed[ai] = ai, true }

	matchUnique := func(key func(pptxTextShape) string, allowEmpty bool) {
		beforeKeys, afterKeys := make(map[string][]int), make(map[string][]int)
		for i, shape := range before {
			if _, ok := beforeToAfter[i]; !ok {
				value := key(shape)
				if allowEmpty || value != "" {
					beforeKeys[value] = append(beforeKeys[value], i)
				}
			}
		}
		for i, shape := range after {
			if !afterUsed[i] {
				value := key(shape)
				if allowEmpty || value != "" {
					afterKeys[value] = append(afterKeys[value], i)
				}
			}
		}
		for value, bi := range beforeKeys {
			ai := afterKeys[value]
			if value == "" || len(bi) != 1 || len(ai) != 1 {
				continue
			}
			beforeShape, afterShape := before[bi[0]], after[ai[0]]
			// A differing available ID identifies a replacement, even when its
			// name or text happens to match the removed shape.
			if beforeShape.ID != "" && afterShape.ID != "" && beforeShape.ID != afterShape.ID {
				continue
			}
			pair(bi[0], ai[0])
		}
	}
	matchUnique(func(shape pptxTextShape) string { return shape.ID }, false)
	matchUnique(func(shape pptxTextShape) string { return shape.Name }, false)
	matchUnique(func(shape pptxTextShape) string { return shape.Text }, false)

	anchors := make([]pptxShapePair, 0, len(beforeToAfter))
	for bi, ai := range beforeToAfter {
		anchors = append(anchors, pptxShapePair{before: bi, after: ai})
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].before < anchors[j].before })
	monotonic := true
	for i := 1; i < len(anchors); i++ {
		if anchors[i-1].after >= anchors[i].after {
			monotonic = false
			break
		}
	}
	if monotonic {
		boundaries := append([]pptxShapePair{{before: -1, after: -1}}, anchors...)
		boundaries = append(boundaries, pptxShapePair{before: len(before), after: len(after)})
		for i := 1; i < len(boundaries); i++ {
			left, right := boundaries[i-1], boundaries[i]
			beforeGap, afterGap := make([]int, 0), make([]int, 0)
			for j := left.before + 1; j < right.before; j++ {
				if _, ok := beforeToAfter[j]; !ok {
					beforeGap = append(beforeGap, j)
				}
			}
			for j := left.after + 1; j < right.after; j++ {
				if !afterUsed[j] {
					afterGap = append(afterGap, j)
				}
			}
			if len(beforeGap) != len(afterGap) {
				continue
			}
			for j := range beforeGap {
				bi, ai := beforeGap[j], afterGap[j]
				// A changed numeric ID is evidence of different shapes. Positional
				// fallback is reserved for cases where at least one ID is absent.
				if before[bi].ID == "" || after[ai].ID == "" {
					pair(bi, ai)
				}
			}
		}
	}

	pairs := make([]pptxShapePair, 0, len(beforeToAfter))
	removed := make([]int, 0)
	for bi := range before {
		if ai, ok := beforeToAfter[bi]; ok {
			pairs = append(pairs, pptxShapePair{before: bi, after: ai})
		} else {
			removed = append(removed, bi)
		}
	}
	added := make([]int, 0)
	for ai, used := range afterUsed {
		if !used {
			added = append(added, ai)
		}
	}
	return pairs, removed, added
}

func pptxChanges(before, after []pptxTextSlide) []PPTXTextChange {
	pairs, removedSlides, addedSlides := matchPPTXSlides(before, after)
	changes := make([]PPTXTextChange, 0)
	appendShapeChange := func(bSlide *pptxTextSlide, aSlide *pptxTextSlide, bShape *pptxTextShape, aShape *pptxTextShape, status string) {
		change := PPTXTextChange{Status: status}
		if bSlide != nil {
			change.BeforeSlide = bSlide.Number
			change.Slide = bSlide.Number
		}
		if aSlide != nil {
			change.AfterSlide = aSlide.Number
			change.Slide = aSlide.Number
		}
		if bShape != nil {
			change.BeforeShapeID = bShape.ID
			change.Before = bShape.Text
		}
		if aShape != nil {
			change.AfterShapeID = aShape.ID
			change.After = aShape.Text
		}
		if bShape != nil && aShape != nil && bShape.ID != "" && bShape.ID == aShape.ID {
			change.ShapeID = bShape.ID
		}
		changes = append(changes, change)
	}
	for _, slidePair := range pairs {
		bSlide, aSlide := &before[slidePair.before], &after[slidePair.after]
		shapePairs, removedShapes, addedShapes := matchPPTXShapes(bSlide.Shapes, aSlide.Shapes)
		for _, shapePair := range shapePairs {
			bShape, aShape := &bSlide.Shapes[shapePair.before], &aSlide.Shapes[shapePair.after]
			if bShape.Text != aShape.Text {
				appendShapeChange(bSlide, aSlide, bShape, aShape, "changed")
			}
		}
		for _, shapeIndex := range removedShapes {
			if bSlide.Shapes[shapeIndex].Text != "" {
				appendShapeChange(bSlide, nil, &bSlide.Shapes[shapeIndex], nil, "removed")
			}
		}
		for _, shapeIndex := range addedShapes {
			if aSlide.Shapes[shapeIndex].Text != "" {
				appendShapeChange(nil, aSlide, nil, &aSlide.Shapes[shapeIndex], "added")
			}
		}
	}
	for _, slideIndex := range removedSlides {
		slide := &before[slideIndex]
		for i := range slide.Shapes {
			if slide.Shapes[i].Text != "" {
				appendShapeChange(slide, nil, &slide.Shapes[i], nil, "removed")
			}
		}
	}
	for _, slideIndex := range addedSlides {
		slide := &after[slideIndex]
		for i := range slide.Shapes {
			if slide.Shapes[i].Text != "" {
				appendShapeChange(nil, slide, nil, &slide.Shapes[i], "added")
			}
		}
	}
	return changes
}

func pptxTextArtifacts(slides []pptxTextSlide) []string {
	var lines []string
	for _, slide := range slides {
		lines = append(lines, fmt.Sprintf("[slide %d id=%s part=%s]\n", slide.Number, slide.ID, slide.Part))
		for _, shape := range slide.Shapes {
			if shape.Text == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("[shape id=%s name=%q]\n", shape.ID, shape.Name))
			appendDocxDiffLines(&lines, shape.Text+"\n")
		}
	}
	return lines
}

func pptxTextDiff(beforeName, afterName string, changes []PPTXTextChange) string {
	var lines []string
	for _, change := range changes {
		slide := fmt.Sprintf("slide %d", change.Slide)
		if change.BeforeSlide != 0 && change.AfterSlide != 0 && change.BeforeSlide != change.AfterSlide {
			slide = fmt.Sprintf("slide %d (before %d, after %d)", change.Slide, change.BeforeSlide, change.AfterSlide)
		}
		shapeID := change.ShapeID
		if shapeID == "" {
			shapeID = change.AfterShapeID
		}
		if shapeID == "" {
			shapeID = change.BeforeShapeID
		}
		detail := fmt.Sprintf("%s %s", slide, change.Status)
		if shapeID != "" {
			detail += " shape id=" + shapeID
		}
		lines = append(lines, "["+detail+"]\n")
		if change.Before != "" || change.Status == "changed" {
			appendDocxDiffLines(&lines, "-"+change.Before+"\n")
		}
		if change.After != "" || change.Status == "changed" {
			appendDocxDiffLines(&lines, "+"+change.After+"\n")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("--- %s\n+++ %s\n", filepath.Base(beforeName), filepath.Base(afterName)) + strings.Join(lines, "")
}
