package wordedit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

var Operations = []string{"section-insert", "header-footer", "header-footer-link", "table-column-insert", "table-column-delete", "table-merge", "table-unmerge", "image-insert", "image-replace", "image-resize", "image-delete", "batch", "replace-text", "paragraph-insert", "paragraph-delete", "paragraph-format", "run-format", "table-cell-text", "table-cell-format", "table-row-insert", "table-row-delete", "section-format", "redline", "redline-insertion", "accept", "reject", "comment", "equation", "accessibility", "google-title", "metadata", "comment-edit", "comments-remove", "bookmark", "reference", "caption", "toc", "navigation", "control", "control-fill", "protection", "merge", "note", "privacy", "materialize", "redact", "normalize", "table-geometry", "freeze-references", "watermark", "watermark-remove", "style-pack", "clone-component"}

// Edit applies one operation from spec to source and writes a new file.
// baseDir resolves relative paths inside the spec (merge files, templates).
func Edit(source, destination string, spec map[string]any, baseDir string) (map[string]any, error) {
	if filepath.Clean(source) == filepath.Clean(destination) {
		return nil, errors.New("output must be a new file, not the original")
	}
	if _, err := os.Stat(destination); err == nil {
		return nil, errors.New("output must be a new file, not the original")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	d, err := Open(raw)
	if err != nil {
		return nil, err
	}
	op := str(spec, "operation")
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}
	var redacted []string
	switch op {
	case "section-insert", "header-footer", "header-footer-link", "table-column-insert", "table-column-delete", "table-merge", "table-unmerge", "image-insert", "image-replace", "image-resize", "image-delete", "batch", "replace-text", "paragraph-insert", "paragraph-delete", "paragraph-format", "run-format", "table-cell-text", "table-cell-format", "table-row-insert", "table-row-delete", "section-format":
		var resolveAssets func(map[string]any)
		resolveAssets = func(s map[string]any) {
			if p := str(s, "path"); p != "" {
				s["path"] = resolve(p)
			}
			if edits, ok := s["edits"].([]any); ok {
				for _, e := range edits {
					if m, ok := e.(map[string]any); ok {
						resolveAssets(m)
					}
				}
			}
		}
		resolveAssets(spec)
		err = generalEdit(d, spec)
	case "redline":
		if _, ok := spec["new"].(string); !ok {
			return nil, errors.New("nonempty old text and author, and string new text are required")
		}
		err = redline(d, str(spec, "old"), str(spec, "new"), str(spec, "author"))
	case "redline-insertion":
		if _, ok := spec["new"].(string); !ok {
			return nil, errors.New("replacement text and author are required")
		}
		err = redlineInsertion(d, spec["id"], str(spec, "new"), str(spec, "author"))
	case "accept", "reject":
		keep := false
		if v, ok := spec["keep_tracking_on"]; ok {
			b, isBool := v.(bool)
			if !isBool {
				return nil, errors.New("keep_tracking_on must be a boolean")
			}
			keep = b
		}
		err = textRevisions(d, op == "accept", keep)
	case "comment":
		err = comment(d, spec)
	case "equation":
		err = equation(d, spec)
	case "accessibility":
		err = accessibility(d, spec)
	case "google-title":
		err = googleTitle(d)
	case "metadata":
		err = metadata(d, spec)
	case "comment-edit":
		err = commentEdit(d, spec)
	case "comments-remove":
		err = commentsRemove(d)
	case "bookmark":
		p, perr := selectParagraph(d, spec["paragraph"])
		if perr != nil {
			return nil, perr
		}
		err = addBookmark(d, p, str(spec, "name"))
	case "reference":
		err = reference(d, spec)
	case "caption":
		err = caption(d, spec)
	case "toc":
		err = toc(d, spec)
	case "navigation":
		err = navigation(d, spec)
	case "control":
		err = control(d, spec)
	case "control-fill":
		err = controlFill(d, spec)
	case "protection":
		err = protection(d, spec)
	case "clone-component":
		path := resolve(str(spec, "path"))
		if path == "" {
			path = source
		}
		err = cloneComponent(d, path, spec)
	case "merge":
		list, _ := spec["files"].([]any)
		var files []string
		for _, v := range list {
			s, _ := v.(string)
			files = append(files, resolve(s))
		}
		err = merge(d, files)
	case "note":
		err = note(d, spec)
	case "privacy":
		err = privacy(d)
	case "materialize":
		var only []string
		if v, ok := spec["only"]; ok {
			list, isList := v.([]any)
			if !isList || len(list) == 0 {
				return nil, errors.New("only must select SEQ and/or REF")
			}
			for _, x := range list {
				s, _ := x.(string)
				only = append(only, s)
			}
		}
		err = materialize(d, only)
	case "redact":
		redacted, err = redact(d, spec)
	case "normalize":
		err = normalize(d, spec)
	case "table-geometry":
		err = tableGeometry(d, spec)
	case "freeze-references":
		err = freezeReferences(d)
	case "watermark":
		err = watermark(d, spec)
	case "watermark-remove":
		err = watermarkRemove(d, spec)
	case "style-pack":
		err = stylePack(d, resolve(str(spec, "template")))
	default:
		return nil, fmt.Errorf("unsupported operation: %s (supported: %s)", op, strings.Join(sorted(Operations), ", "))
	}
	if err != nil {
		return nil, err
	}
	data, err := d.Bytes()
	if err != nil {
		return nil, err
	}
	if op == "redact" {
		p, _ := opc.Read(data)
		for _, part := range p.Parts {
			for _, v := range redacted {
				if bytes.Contains(part.Data, []byte(v)) || bytes.Contains(part.Data, utf16le(v)) {
					return nil, errors.New("redaction left matching bytes in " + part.Name)
				}
			}
		}
	}
	if err := opc.WriteNew(destination, data); err != nil {
		return nil, err
	}
	return map[string]any{"output": destination, "operation": op, "visual_review_required": true}, nil
}

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func sorted(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}
