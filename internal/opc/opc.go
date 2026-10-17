// Package opc reads and writes Office Open XML packages while preserving
// unrelated parts byte-for-byte.
package opc

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/beevik/etree"
)

const (
	MaxParts      = 20000
	MaxTotalBytes = 256 << 20
	MaxPartBytes  = 128 << 20
)

const (
	NSRel  = "http://schemas.openxmlformats.org/package/2006/relationships"
	NSCT   = "http://schemas.openxmlformats.org/package/2006/content-types"
	NSOffR = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// Part retains original ZIP metadata; serialization may normalize compression.
// Preservation guarantees concern uncompressed part bytes, not archive encoding.
type Part struct {
	Name   string
	Data   []byte
	Header zip.FileHeader
}

type Package struct {
	Parts []*Part
	index map[string]*Part
}

func Open(file string) (*Package, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Read(raw)
}

func Read(raw []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("not a valid Office package (zip): %w", err)
	}
	if len(zr.File) > MaxParts {
		return nil, errors.New("package exceeds inspection limits; nothing was inspected")
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
	}
	if total > MaxTotalBytes {
		return nil, errors.New("package exceeds inspection limits; nothing was inspected")
	}
	p := &Package{index: map[string]*Part{}}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") {
			return nil, fmt.Errorf("unsafe part name: %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, MaxPartBytes+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if len(data) > MaxPartBytes {
			return nil, fmt.Errorf("part too large: %s", f.Name)
		}
		if _, dup := p.index[f.Name]; dup {
			return nil, fmt.Errorf("duplicate part: %s", f.Name)
		}
		part := &Part{Name: f.Name, Data: data, Header: f.FileHeader}
		p.Parts = append(p.Parts, part)
		p.index[f.Name] = part
	}
	return p, nil
}

func (p *Package) Has(name string) bool { _, ok := p.index[name]; return ok }

func (p *Package) Get(name string) []byte {
	if part := p.index[name]; part != nil {
		return part.Data
	}
	return nil
}

func (p *Package) Names() []string {
	out := make([]string, 0, len(p.Parts))
	for _, part := range p.Parts {
		out = append(out, part.Name)
	}
	return out
}

// Set replaces or appends a part.
func (p *Package) Set(name string, data []byte) {
	if part := p.index[name]; part != nil {
		part.Data = data
		return
	}
	part := &Part{Name: name, Data: data, Header: zip.FileHeader{Name: name, Method: zip.Deflate}}
	p.Parts = append(p.Parts, part)
	p.index[name] = part
}

func (p *Package) Remove(name string) {
	if p.index[name] == nil {
		return
	}
	delete(p.index, name)
	for i, part := range p.Parts {
		if part.Name == name {
			p.Parts = append(p.Parts[:i], p.Parts[i+1:]...)
			break
		}
	}
}

func (p *Package) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range p.Parts {
		h := part.Header
		h.Name = part.Name
		// LibreOffice cannot reliably read stored OPC members with streaming
		// data descriptors (notably _rels/.rels in imported presentations).
		// Deflate all members; uncompressed part bytes remain unchanged.
		h.Method = zip.Deflate
		h.CompressedSize64, h.UncompressedSize64, h.CRC32 = 0, 0, 0
		h.Extra = nil
		w, err := zw.CreateHeader(&h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(part.Data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CheckXML rejects DTD/entity declarations, which Office parts never need.
func CheckXML(data []byte) error {
	if bytes.Contains(data, []byte("<!DOCTYPE")) || bytes.Contains(data, []byte("<!ENTITY")) {
		return errors.New("XML entity declarations are not supported")
	}
	return nil
}

func ParseXML(data []byte) (*etree.Document, error) {
	if err := CheckXML(data); err != nil {
		return nil, err
	}
	doc := etree.NewDocument()
	doc.ReadSettings.Permissive = false
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, err
	}
	if doc.Root() == nil {
		return nil, errors.New("empty XML part")
	}
	return doc, nil
}

func (p *Package) XML(name string) (*etree.Document, error) {
	data := p.Get(name)
	if data == nil {
		return nil, fmt.Errorf("missing part: %s", name)
	}
	doc, err := ParseXML(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return doc, nil
}

func (p *Package) SetXML(name string, doc *etree.Document) error {
	doc.WriteSettings.CanonicalEndTags = false
	data, err := doc.WriteToBytes()
	if err != nil {
		return err
	}
	// etree retains a UTF-8 BOM as leading character data. Normalize it before
	// deciding whether an XML declaration is already present.
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !bytes.HasPrefix(data, []byte("<?xml")) {
		data = append([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"), data...)
	}
	p.Set(name, data)
	return nil
}

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type Relationship struct {
	ID, Type, Target, TargetMode string
}

func RelsName(part string) string {
	return path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
}

// SourceOf is the part owning a .rels file ("" for the package root).
func SourceOf(rels string) string {
	dir := path.Dir(path.Dir(rels))
	base := strings.TrimSuffix(path.Base(rels), ".rels")
	if dir == "." {
		dir = ""
	}
	if base == "" {
		return ""
	}
	return path.Join(dir, base)
}

func (p *Package) Rels(part string) ([]Relationship, error) {
	name := RelsName(part)
	if part == "" {
		name = "_rels/.rels"
	}
	if !p.Has(name) {
		return nil, nil
	}
	doc, err := p.XML(name)
	if err != nil {
		return nil, err
	}
	var out []Relationship
	for _, e := range doc.Root().ChildElements() {
		out = append(out, Relationship{e.SelectAttrValue("Id", ""), e.SelectAttrValue("Type", ""), e.SelectAttrValue("Target", ""), e.SelectAttrValue("TargetMode", "")})
	}
	return out, nil
}

// Resolve maps a relationship target to an absolute part name.
func Resolve(source, target string) string {
	target = strings.SplitN(target, "#", 2)[0]
	if t, err := url.PathUnescape(target); err == nil {
		target = t
	}
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return strings.TrimPrefix(path.Clean(path.Join(path.Dir(source), target)), "/")
}

// AddRel appends a relationship to part and returns its new ID.
func (p *Package) AddRel(part, typ, target string, external bool) (string, error) {
	name := RelsName(part)
	var doc *etree.Document
	if p.Has(name) {
		var err error
		if doc, err = p.XML(name); err != nil {
			return "", err
		}
	} else {
		doc = etree.NewDocument()
		doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		doc.CreateElement("Relationships").CreateAttr("xmlns", NSRel)
	}
	used := map[string]bool{}
	for _, e := range doc.Root().ChildElements() {
		used[e.SelectAttrValue("Id", "")] = true
	}
	id := ""
	for i := 1; ; i++ {
		id = fmt.Sprintf("rId%d", i)
		if !used[id] {
			break
		}
	}
	r := doc.Root().CreateElement("Relationship")
	r.CreateAttr("Id", id)
	r.CreateAttr("Type", typ)
	r.CreateAttr("Target", target)
	if external {
		r.CreateAttr("TargetMode", "External")
	}
	return id, p.SetXML(name, doc)
}

// EnsureContentType registers an override (or default for extension when partName starts with ".").
func (p *Package) EnsureContentType(partName, contentType string) error {
	doc, err := p.XML("[Content_Types].xml")
	if err != nil {
		return err
	}
	root := doc.Root()
	if strings.HasPrefix(partName, ".") {
		ext := strings.TrimPrefix(partName, ".")
		for _, e := range root.SelectElements("Default") {
			if strings.EqualFold(e.SelectAttrValue("Extension", ""), ext) {
				return nil
			}
		}
		d := etree.NewElement("Default")
		d.CreateAttr("Extension", ext)
		d.CreateAttr("ContentType", contentType)
		root.InsertChildAt(0, d)
		return p.SetXML("[Content_Types].xml", doc)
	}
	pn := "/" + strings.TrimPrefix(partName, "/")
	for _, e := range root.SelectElements("Override") {
		if e.SelectAttrValue("PartName", "") == pn {
			e.RemoveAttr("ContentType")
			e.CreateAttr("ContentType", contentType)
			return p.SetXML("[Content_Types].xml", doc)
		}
	}
	o := root.CreateElement("Override")
	o.CreateAttr("PartName", pn)
	o.CreateAttr("ContentType", contentType)
	return p.SetXML("[Content_Types].xml", doc)
}

func (p *Package) ContentType(name string) string {
	doc, err := p.XML("[Content_Types].xml")
	if err != nil {
		return ""
	}
	for _, e := range doc.Root().SelectElements("Override") {
		if strings.TrimPrefix(e.SelectAttrValue("PartName", ""), "/") == name {
			return e.SelectAttrValue("ContentType", "")
		}
	}
	ext := strings.TrimPrefix(path.Ext(name), ".")
	for _, e := range doc.Root().SelectElements("Default") {
		if strings.EqualFold(e.SelectAttrValue("Extension", ""), ext) {
			return e.SelectAttrValue("ContentType", "")
		}
	}
	return ""
}

func (p *Package) SortedNames() []string {
	n := p.Names()
	sort.Strings(n)
	return n
}

// WriteNew writes data to a file that must not exist yet (O_EXCL).
func WriteNew(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output already exists: %s: %w", file, os.ErrExist)
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(file)
		return err
	}
	return f.Close()
}

// SaveNew serializes the package and writes it to a new file.
func (p *Package) SaveNew(file string) error {
	data, err := p.Bytes()
	if err != nil {
		return err
	}
	if _, err := Read(data); err != nil {
		return fmt.Errorf("refusing to write invalid package: %w", err)
	}
	return WriteNew(file, data)
}
