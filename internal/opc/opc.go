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
	"hash/crc32"
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
	src    *rawSource
}

// rawSource is the original compressed member, reused by Bytes only while
// Data still matches it (same length and CRC-32).
type rawSource struct {
	compressed []byte
	header     zip.FileHeader
}

type Package struct {
	Parts   []*Part
	index   map[string]*Part
	ctCache *contentTypes
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
	// Private copy so later caller mutations of raw cannot leak into Bytes.
	retained := bytes.Clone(raw)
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
		part := &Part{Name: f.Name, Data: data, Header: f.FileHeader, src: rawSourceOf(retained, f)}
		p.Parts = append(p.Parts, part)
		p.index[f.Name] = part
	}
	return p, nil
}

func (p *Package) Has(name string) bool { _, ok := p.index[name]; return ok }

// UnusedName returns prefix+n+suffix for the lowest n >= 1 not present in the package.
func (p *Package) UnusedName(prefix, suffix string) string {
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s%d%s", prefix, i, suffix)
		if !p.Has(n) {
			return n
		}
	}
}

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
	if name == contentTypesName {
		p.ctCache = nil
	}
	if part := p.index[name]; part != nil {
		part.Data = data
		part.src = nil
		return
	}
	part := &Part{Name: name, Data: data, Header: zip.FileHeader{Name: name, Method: zip.Deflate}}
	p.Parts = append(p.Parts, part)
	p.index[name] = part
}

func (p *Package) Remove(name string) {
	if name == contentTypesName {
		p.ctCache = nil
	}
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

// rawSourceOf aliases f's compressed bytes within raw when they can be copied
// verbatim. Stored members with data descriptors are excluded because
// LibreOffice mishandles them; Bytes re-deflates those.
func rawSourceOf(raw []byte, f *zip.File) *rawSource {
	switch {
	case f.Flags&0x1 != 0:
		return nil
	case f.Method == zip.Deflate:
	case f.Method == zip.Store && f.Flags&0x8 == 0 && f.CompressedSize64 == f.UncompressedSize64:
	default:
		return nil
	}
	off, err := f.DataOffset()
	if err != nil || off < 0 || f.CompressedSize64 > uint64(len(raw)) || uint64(off) > uint64(len(raw))-f.CompressedSize64 {
		return nil
	}
	h := f.FileHeader
	h.Flags &^= 0x8
	h.Extra = nil
	return &rawSource{compressed: raw[off : uint64(off)+f.CompressedSize64 : uint64(off)+f.CompressedSize64], header: h}
}

func (part *Part) rawCopyable() bool {
	s := part.src
	return s != nil && part.Name == s.header.Name &&
		uint64(len(part.Data)) == s.header.UncompressedSize64 &&
		crc32.ChecksumIEEE(part.Data) == s.header.CRC32
}

// Bytes serializes the package. Unmodified members are copied in their
// original compressed form; all other members are deflated.
func (p *Package) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range p.Parts {
		if part.rawCopyable() {
			h := part.Header
			s := part.src
			h.Name, h.Method, h.Flags = s.header.Name, s.header.Method, s.header.Flags
			h.CRC32, h.CompressedSize64, h.UncompressedSize64 = s.header.CRC32, s.header.CompressedSize64, s.header.UncompressedSize64
			h.Extra = nil
			w, err := zw.CreateRaw(&h)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(s.compressed); err != nil {
				return nil, err
			}
			continue
		}
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

const contentTypesName = "[Content_Types].xml"

// contentTypes is a parsed view of [Content_Types].xml, keyed on the part
// and data slice it was built from so replacing Part.Data invalidates it.
// In-place byte edits of the existing slice are not detected; use Set.
type contentTypes struct {
	part      *Part
	data      *byte
	size      int
	valid     bool
	overrides map[string]string
	defaults  map[string]string
}

func foldExt(ext string) string { return strings.ToLower(strings.ToUpper(ext)) }

func (p *Package) contentTypes() *contentTypes {
	part := p.index[contentTypesName]
	var data *byte
	size := 0
	if part != nil {
		size = len(part.Data)
		if cap(part.Data) > 0 {
			data = &part.Data[:1][0]
		}
	}
	if c := p.ctCache; c != nil && c.part == part && c.data == data && c.size == size {
		return c
	}
	c := &contentTypes{part: part, data: data, size: size}
	p.ctCache = c
	if part == nil || part.Data == nil {
		return c
	}
	doc, err := ParseXML(part.Data)
	if err != nil {
		return c
	}
	c.valid = true
	c.overrides = map[string]string{}
	c.defaults = map[string]string{}
	for _, e := range doc.Root().SelectElements("Override") {
		k := strings.TrimPrefix(e.SelectAttrValue("PartName", ""), "/")
		if _, ok := c.overrides[k]; !ok {
			c.overrides[k] = e.SelectAttrValue("ContentType", "")
		}
	}
	for _, e := range doc.Root().SelectElements("Default") {
		k := foldExt(e.SelectAttrValue("Extension", ""))
		if _, ok := c.defaults[k]; !ok {
			c.defaults[k] = e.SelectAttrValue("ContentType", "")
		}
	}
	return c
}

func (p *Package) ContentType(name string) string {
	c := p.contentTypes()
	if !c.valid {
		return ""
	}
	if ct, ok := c.overrides[name]; ok {
		return ct
	}
	return c.defaults[foldExt(strings.TrimPrefix(path.Ext(name), "."))]
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
