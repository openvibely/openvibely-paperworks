package office

import (
	"slices"

	"github.com/beevik/etree"
)

// NewChild creates ns:local under parent using the prefix already bound to ns
// (falling back to the parent's own prefix), placed before the first existing
// child whose local name appears in later. Pass nil later to append.
func NewChild(parent *etree.Element, ns, local string, later []string) *etree.Element {
	prefix := nsPrefix(parent, ns)
	if prefix == "" && parent.NamespaceURI() == ns {
		prefix = parent.Space
	}
	e := etree.NewElement(qual(prefix, local))
	InsertBefore(parent, e, later)
	return e
}

// InsertBefore inserts e before the first child of parent whose local name
// is listed in later, or appends it.
func InsertBefore(parent, e *etree.Element, later []string) {
	for _, c := range parent.ChildElements() {
		if slices.Contains(later, c.Tag) {
			parent.InsertChildAt(c.Index(), e)
			return
		}
	}
	parent.AddChild(e)
}

// After returns the names following name in an ordered schema sequence.
func After(order []string, name string) []string {
	i := slices.Index(order, name)
	if i < 0 {
		return nil
	}
	return order[i+1:]
}

// EnsureChild returns the first ns:local child, creating it in schema order.
func EnsureChild(parent *etree.Element, ns, local string, order []string) *etree.Element {
	if c := Child(parent, ns, local); c != nil {
		return c
	}
	return NewChild(parent, ns, local, After(order, local))
}

func SetAttr(e *etree.Element, key, value string) { e.CreateAttr(key, value) }

// RemoveAll removes every direct child matching ns:local.
func RemoveAll(parent *etree.Element, ns, local string) {
	for _, c := range Children(parent, ns, local) {
		parent.RemoveChild(c)
	}
}

func Truthy(v string, def bool) bool {
	if v == "" {
		return def
	}
	return v == "1" || v == "true"
}

// WorksheetOrder is the CT_Worksheet child sequence.
var WorksheetOrder = []string{"sheetPr", "dimension", "sheetViews", "sheetFormatPr", "cols", "sheetData", "sheetCalcPr", "sheetProtection", "protectedRanges", "scenarios", "autoFilter", "sortState", "dataConsolidate", "customSheetViews", "mergeCells", "phoneticPr", "conditionalFormatting", "dataValidations", "hyperlinks", "printOptions", "pageMargins", "pageSetup", "headerFooter", "rowBreaks", "colBreaks", "customProperties", "cellWatches", "ignoredErrors", "smartTags", "drawing", "legacyDrawing", "legacyDrawingHF", "drawingHF", "picture", "oleObjects", "controls", "webPublishItems", "tableParts", "extLst"}

// WorkbookOrder is the CT_Workbook child sequence.
var WorkbookOrder = []string{"fileVersion", "fileSharing", "workbookPr", "workbookProtection", "bookViews", "sheets", "functionGroups", "externalReferences", "definedNames", "calcPr", "oleSize", "customWorkbookViews", "pivotCaches", "smartTagPr", "smartTagTypes", "webPublishing", "fileRecoveryPr", "webPublishObjects", "extLst"}

// HasAttr returns an unqualified attribute's value and whether it is present.
func HasAttr(e *etree.Element, key string) (string, bool) {
	if e == nil {
		return "", false
	}
	if a := e.SelectAttr(key); a != nil {
		return a.Value, true
	}
	return "", false
}

// AttrOr is a nil-safe SelectAttrValue.
func AttrOr(e *etree.Element, key, def string) string {
	if e == nil {
		return def
	}
	return e.SelectAttrValue(key, def)
}
