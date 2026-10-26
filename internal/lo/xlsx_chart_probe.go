package lo

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	chartContentType   = "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"
	chartExContentType = "application/vnd.ms-office.chartex+xml"
)

// xlsxChartParts reports whether a package has chart or chartEx parts using
// only its content-types manifest and relationship parts. A false second
// result means that package metadata was missing, malformed, or ambiguous.
// Proven chartless packages have every member streamed to discard so ZIP
// decompression and CRC errors are still detected without retaining expanded
// contents in memory.
func xlsxChartParts(src string) (hasCharts, proven bool) {
	info, err := os.Stat(src)
	if err != nil || info.Size() < 0 || info.Size() > maxRefreshPackage {
		return false, false
	}
	archive, err := zip.OpenReader(src)
	if err != nil {
		return false, false
	}
	defer archive.Close()

	if len(archive.File) > opc.MaxParts {
		return false, false
	}
	parts := make(map[string]*zip.File, len(archive.File))
	seen := make(map[string]struct{}, len(archive.File))
	var expanded uint64
	for _, part := range archive.File {
		if part.UncompressedSize64 > uint64(opc.MaxTotalBytes)-expanded {
			return false, false
		}
		expanded += part.UncompressedSize64
		if part.UncompressedSize64 > opc.MaxPartBytes {
			return false, false
		}
		if strings.Contains(part.Name, "..") || strings.HasPrefix(part.Name, "/") || strings.Contains(part.Name, "\\") {
			return false, false
		}
		if _, duplicate := seen[part.Name]; duplicate {
			return false, false
		}
		seen[part.Name] = struct{}{}
		parts[part.Name] = part
	}

	manifest, ok := readRefreshMetadata(parts["[Content_Types].xml"])
	if !ok {
		return false, false
	}
	overrides, defaults, ok := parseRefreshContentTypes(manifest, parts)
	if !ok {
		return false, false
	}

	chart := false
	for name, part := range parts {
		if strings.HasSuffix(name, "/") {
			continue
		}
		contentType := overrides[name]
		if contentType == "" {
			ext := strings.TrimPrefix(path.Ext(name), ".")
			contentType = defaults[strings.ToLower(ext)]
		}
		if contentType == "" {
			return false, false
		}
		if isChartContentType(contentType) {
			chart = true
		}
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		relsXML, ok := readRefreshMetadata(part)
		if !ok {
			return false, false
		}
		relChart, ok := refreshRelationshipsContainChart(relsXML, name, parts)
		if !ok {
			return false, false
		}
		chart = chart || relChart
	}
	if chart {
		return true, true
	}

	// opc.Open validates every member's decompression and CRC but retains all
	// expanded data. The no-chart fast path needs the same safety checks, so
	// stream each member through the ZIP reader and discard the bytes. Reuse a
	// large buffer so highly compressed parts do not devolve into tiny reads.
	buffer := make([]byte, 1<<20)
	for _, part := range archive.File {
		if strings.HasSuffix(part.Name, "/") {
			continue
		}
		reader, err := part.Open()
		if err != nil {
			return false, false
		}
		var n uint64
		for {
			count, readErr := reader.Read(buffer)
			n += uint64(count)
			if n > opc.MaxPartBytes {
				reader.Close()
				return false, false
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				reader.Close()
				return false, false
			}
		}
		closeErr := reader.Close()
		if closeErr != nil || n != part.UncompressedSize64 {
			return false, false
		}
	}
	return false, true
}

func readRefreshMetadata(part *zip.File) ([]byte, bool) {
	if part == nil || part.UncompressedSize64 > maxRefreshXMLPart {
		return nil, false
	}
	reader, err := part.Open()
	if err != nil {
		return nil, false
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, maxRefreshXMLPart+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(data) > maxRefreshXMLPart || uint64(len(data)) != part.UncompressedSize64 {
		return nil, false
	}
	return data, true
}

func parseRefreshContentTypes(data []byte, parts map[string]*zip.File) (map[string]string, map[string]string, bool) {
	doc, err := opc.ParseXML(data)
	if err != nil || doc.Root().NamespaceURI() != opc.NSCT || doc.Root().Tag != "Types" {
		return nil, nil, false
	}
	overrides := make(map[string]string)
	defaults := make(map[string]string)
	for _, item := range doc.Root().ChildElements() {
		switch item.Tag {
		case "Default":
			if item.NamespaceURI() != opc.NSCT {
				return nil, nil, false
			}
			ext := strings.ToLower(item.SelectAttrValue("Extension", ""))
			contentType := item.SelectAttrValue("ContentType", "")
			if ext == "" || contentType == "" || strings.Contains(ext, "/") {
				return nil, nil, false
			}
			if _, exists := defaults[ext]; exists {
				return nil, nil, false
			}
			defaults[ext] = contentType
		case "Override":
			if item.NamespaceURI() != opc.NSCT {
				return nil, nil, false
			}
			name := strings.TrimPrefix(item.SelectAttrValue("PartName", ""), "/")
			contentType := item.SelectAttrValue("ContentType", "")
			if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || contentType == "" {
				return nil, nil, false
			}
			if _, exists := overrides[name]; exists {
				return nil, nil, false
			}
			if parts[name] == nil {
				return nil, nil, false
			}
			overrides[name] = contentType
		default:
			return nil, nil, false
		}
	}
	return overrides, defaults, true
}

func refreshRelationshipsContainChart(data []byte, relsName string, parts map[string]*zip.File) (bool, bool) {
	doc, err := opc.ParseXML(data)
	if err != nil || doc.Root().NamespaceURI() != opc.NSRel || doc.Root().Tag != "Relationships" {
		return false, false
	}
	owner := opc.SourceOf(relsName)
	seenIDs := make(map[string]struct{}, len(doc.Root().ChildElements()))
	hasChart := false
	for _, rel := range doc.Root().ChildElements() {
		if rel.NamespaceURI() != opc.NSRel || rel.Tag != "Relationship" {
			return false, false
		}
		id := rel.SelectAttrValue("Id", "")
		typ := rel.SelectAttrValue("Type", "")
		target := rel.SelectAttrValue("Target", "")
		mode := rel.SelectAttrValue("TargetMode", "")
		if id == "" || typ == "" || target == "" {
			return false, false
		}
		if _, duplicate := seenIDs[id]; duplicate {
			return false, false
		}
		seenIDs[id] = struct{}{}
		lastSlash := strings.LastIndex(typ, "/")
		if lastSlash < 0 || (typ[lastSlash+1:] != "chart" && typ[lastSlash+1:] != "chartEx") {
			continue
		}
		if mode != "" {
			return false, false
		}
		name := opc.Resolve(owner, target)
		if name == "" || path.IsAbs(name) || strings.Contains(name, "..") || parts[name] == nil {
			return false, false
		}
		hasChart = true
	}
	return hasChart, true
}

func isChartContentType(contentType string) bool {
	switch strings.ToLower(contentType) {
	case chartContentType, chartExContentType:
		return true
	default:
		return false
	}
}
