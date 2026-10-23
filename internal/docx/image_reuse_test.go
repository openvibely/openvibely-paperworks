package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func testImageBytes(t *testing.T, format string, variant uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{R: variant + uint8(x*17), G: uint8(y*61) + variant, B: uint8(x*y*29) + variant, A: 255})
		}
	}
	var out bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&out, img)
	case "jpg":
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 90})
	case "gif":
		err = gif.Encode(&out, img, nil)
	default:
		t.Fatalf("unsupported test image format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func writeTestImage(t *testing.T, name string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

type imagePlacementInfo struct {
	id, description string
	cx, cy          int64
	relID           string
}

func inspectImagePlacements(t *testing.T, data []byte) ([]imagePlacementInfo, *opc.Package) {
	t.Helper()
	pkg, err := opc.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var placements []imagePlacementInfo
	var current *imagePlacementInfo
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "inline":
				placements = append(placements, imagePlacementInfo{})
				current = &placements[len(placements)-1]
			case "docPr":
				if current == nil {
					continue
				}
				for _, attr := range element.Attr {
					switch attr.Name.Local {
					case "id":
						current.id = attr.Value
					case "descr":
						current.description = attr.Value
					}
				}
			case "extent":
				if current != nil {
					for _, attr := range element.Attr {
						switch attr.Name.Local {
						case "cx":
							current.cx, err = strconv.ParseInt(attr.Value, 10, 64)
						case "cy":
							current.cy, err = strconv.ParseInt(attr.Value, 10, 64)
						}
						if err != nil {
							t.Fatal(err)
						}
					}
				}
			case "blip":
				if current != nil {
					for _, attr := range element.Attr {
						if attr.Name.Local == "embed" {
							current.relID = attr.Value
						}
					}
				}
			}
		case xml.EndElement:
			if element.Name.Local == "drawing" {
				current = nil
			}
		}
	}
	return placements, pkg
}

func assertImagePlacements(t *testing.T, spec Spec, baseDir string, inputs [][]byte, uniqueMedia int) {
	t.Helper()
	data, err := Build(spec, baseDir)
	if err != nil {
		t.Fatal(err)
	}
	placements, pkg := inspectImagePlacements(t, data)
	if len(placements) != len(spec.Blocks) {
		t.Fatalf("got %d drawings, want %d", len(placements), len(spec.Blocks))
	}
	media := make([]string, 0, uniqueMedia)
	for _, part := range pkg.Parts {
		if strings.HasPrefix(part.Name, "word/media/") {
			media = append(media, part.Name)
		}
	}
	if len(media) != uniqueMedia {
		t.Fatalf("got %d media parts, want %d: %v", len(media), uniqueMedia, media)
	}

	rels, err := pkg.Rels("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]opc.Relationship)
	imageRelationships := 0
	for _, rel := range rels {
		byID[rel.ID] = rel
		if rel.Type == opc.NSOffR+"/image" {
			imageRelationships++
		}
	}
	if imageRelationships != uniqueMedia {
		t.Fatalf("got %d image relationships, want %d", imageRelationships, uniqueMedia)
	}
	for _, part := range pkg.Parts {
		if !strings.HasSuffix(part.Name, ".rels") {
			continue
		}
		relationships, err := pkg.Rels(opc.SourceOf(part.Name))
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]bool, len(relationships))
		for _, rel := range relationships {
			if seen[rel.ID] {
				t.Errorf("duplicate relationship ID %q in %s", rel.ID, part.Name)
			}
			seen[rel.ID] = true
			if rel.TargetMode != "External" && !pkg.Has(opc.Resolve(opc.SourceOf(part.Name), rel.Target)) {
				t.Errorf("relationship target is missing in %s: %+v", part.Name, rel)
			}
		}
	}
	mediaContents := make(map[string][]byte, len(media))
	for _, name := range media {
		mediaContents[name] = pkg.Get(name)
		if pkg.ContentType(name) == "" {
			t.Fatalf("missing content type for %s", name)
		}
		matchesInput := false
		for _, input := range inputs {
			if bytes.Equal(mediaContents[name], input) {
				matchesInput = true
				break
			}
		}
		if !matchesInput {
			t.Errorf("media payload %s does not match any authored image", name)
		}
	}

	seenIDs := map[string]bool{}
	for i, got := range placements {
		block := spec.Blocks[i]
		if seenIDs[got.id] {
			t.Errorf("placement %d reused drawing identifier %q", i, got.id)
		}
		seenIDs[got.id] = true
		if got.description != block.Description {
			t.Errorf("placement %d description = %q, want %q", i, got.description, block.Description)
		}
		width := block.WidthIn
		if width <= 0 {
			width = min(6, float64((8.5-2)*1440)/1440)
		}
		wantCX := int64(width * 914400)
		cfg, _, err := image.DecodeConfig(bytes.NewReader(inputs[i]))
		if err != nil {
			t.Fatal(err)
		}
		wantCY := wantCX * int64(cfg.Height) / int64(max(cfg.Width, 1))
		if got.cx != wantCX || got.cy != wantCY {
			t.Errorf("placement %d extent = (%d,%d), want (%d,%d)", i, got.cx, got.cy, wantCX, wantCY)
		}
		rel, ok := byID[got.relID]
		if !ok || rel.Type != opc.NSOffR+"/image" {
			t.Errorf("placement %d has unresolved image relationship %q", i, got.relID)
			continue
		}
		target := opc.Resolve("word/document.xml", rel.Target)
		if !pkg.Has(target) {
			t.Errorf("placement %d relationship target %q is missing", i, target)
		}
		if _, ok := mediaContents[target]; !ok {
			t.Errorf("placement %d relationship target %q is not an image media part", i, target)
		}
	}
}

func TestBuildReusesIdenticalImageContent(t *testing.T) {
	dir := t.TempDir()
	pngData := testImageBytes(t, "png", 1)
	jpegData := testImageBytes(t, "jpg", 2)
	gifData := testImageBytes(t, "gif", 3)
	otherPNG := testImageBytes(t, "png", 100)

	t.Run("repeated relative path", func(t *testing.T) {
		writeTestImage(t, filepath.Join(dir, "relative", "same.png"), pngData)
		paths := []string{"same.png", "same.png", "same.png"}
		blocks, inputs := placementBlocks(paths, pngData, pngData, pngData)
		assertImagePlacements(t, Spec{Blocks: blocks}, filepath.Join(dir, "relative"), inputs, 1)
	})

	t.Run("repeated absolute path", func(t *testing.T) {
		path := writeTestImage(t, filepath.Join(dir, "absolute", "same.png"), pngData)
		blocks, inputs := placementBlocks([]string{path, path}, pngData, pngData)
		assertImagePlacements(t, Spec{Blocks: blocks}, "", inputs, 1)
	})

	t.Run("identical bytes at different paths", func(t *testing.T) {
		first := writeTestImage(t, filepath.Join(dir, "copies", "one.png"), pngData)
		second := writeTestImage(t, filepath.Join(dir, "copies", "two.png"), bytes.Clone(pngData))
		blocks, inputs := placementBlocks([]string{first, second}, pngData, pngData)
		assertImagePlacements(t, Spec{Blocks: blocks}, "", inputs, 1)
	})

	t.Run("different bytes with same filename", func(t *testing.T) {
		first := writeTestImage(t, filepath.Join(dir, "named-a", "image.png"), pngData)
		second := writeTestImage(t, filepath.Join(dir, "named-b", "image.png"), otherPNG)
		blocks, inputs := placementBlocks([]string{first, second}, pngData, otherPNG)
		assertImagePlacements(t, Spec{Blocks: blocks}, "", inputs, 2)
	})

	t.Run("digest collision requires byte equality", func(t *testing.T) {
		collision := imageWithSameDigest(t, pngData)
		if bytes.Equal(collision, pngData) || crc32.Checksum(collision, imageDigestTable) != crc32.Checksum(pngData, imageDigestTable) {
			t.Fatal("test did not construct distinct bytes with a matching digest")
		}
		first := writeTestImage(t, filepath.Join(dir, "collision", "first.png"), pngData)
		second := writeTestImage(t, filepath.Join(dir, "collision", "second.png"), collision)
		blocks, inputs := placementBlocks([]string{first, second}, pngData, collision)
		assertImagePlacements(t, Spec{Blocks: blocks}, "", inputs, 2)
	})

	t.Run("PNG JPEG and GIF", func(t *testing.T) {
		formats := []struct {
			name string
			data []byte
		}{
			{name: "png", data: pngData},
			{name: "jpeg", data: jpegData},
			{name: "gif", data: gifData},
		}
		var paths []string
		var dataSets [][]byte
		for _, format := range formats {
			path := writeTestImage(t, filepath.Join(dir, "formats", format.name+".img"), format.data)
			paths = append(paths, path, path)
			dataSets = append(dataSets, format.data, format.data)
		}
		blocks, inputs := placementBlocks(paths, dataSets...)
		assertImagePlacements(t, Spec{Blocks: blocks}, "", inputs, 3)
		pkgData, err := Build(Spec{Blocks: blocks}, "")
		if err != nil {
			t.Fatal(err)
		}
		_, pkg := inspectImagePlacements(t, pkgData)
		wantTypes := map[string]string{"png": "image/png", "jpg": "image/jpeg", "gif": "image/gif"}
		for ext, contentType := range wantTypes {
			name := ""
			for _, part := range pkg.Parts {
				if strings.HasPrefix(part.Name, "word/media/") && filepath.Ext(part.Name) == "."+ext {
					name = part.Name
				}
			}
			if name == "" || pkg.ContentType(name) != contentType {
				t.Errorf("content type for .%s = %q, want %q", ext, pkg.ContentType(name), contentType)
			}
		}
	})
}

func imageWithSameDigest(t *testing.T, data []byte) []byte {
	t.Helper()
	const patchBytes = 4
	zeroPatch := make([]byte, len(data)+patchBytes)
	copy(zeroPatch, data)
	base := crc32.Checksum(zeroPatch, imageDigestTable)
	columns := [32]uint32{}
	for bit := range 32 {
		candidate := bytes.Clone(zeroPatch)
		candidate[len(data)+bit/8] ^= 1 << uint(bit%8)
		columns[bit] = crc32.Checksum(candidate, imageDigestTable) ^ base
	}

	// Solve the 32-bit CRC patch as a binary linear system.
	var rows [32]uint64
	for outputBit := range 32 {
		for variable, column := range columns {
			if column>>uint(outputBit)&1 != 0 {
				rows[outputBit] |= uint64(1) << uint(variable)
			}
		}
		if (base^crc32.Checksum(data, imageDigestTable))>>uint(outputBit)&1 != 0 {
			rows[outputBit] |= uint64(1) << 32
		}
	}
	pivotRow := 0
	for variable := range 32 {
		pivot := pivotRow
		for pivot < 32 && rows[pivot]>>uint(variable)&1 == 0 {
			pivot++
		}
		if pivot == 32 {
			t.Fatal("CRC patch matrix is singular")
		}
		rows[pivotRow], rows[pivot] = rows[pivot], rows[pivotRow]
		for row := range 32 {
			if row != pivotRow && rows[row]>>uint(variable)&1 != 0 {
				rows[row] ^= rows[pivotRow]
			}
		}
		pivotRow++
	}

	collision := bytes.Clone(data)
	patch := make([]byte, patchBytes)
	for variable := range 32 {
		if rows[variable]>>32&1 != 0 {
			patch[variable/8] |= 1 << uint(variable%8)
		}
	}
	collision = append(collision, patch...)
	if crc32.Checksum(collision, imageDigestTable) != crc32.Checksum(data, imageDigestTable) {
		t.Fatal("failed to construct matching CRC digest")
	}
	return collision
}

func placementBlocks(paths []string, data ...[]byte) ([]Block, [][]byte) {
	blocks := make([]Block, len(paths))
	inputs := make([][]byte, len(paths))
	for i, path := range paths {
		blocks[i] = Block{Type: "image", Path: path, WidthIn: float64(i+1) / 2, Description: "alt & image " + strconv.Itoa(i)}
		inputs[i] = data[i]
	}
	return blocks, inputs
}

func TestBuildImageErrorsRemainErrors(t *testing.T) {
	dir := t.TempDir()
	invalid := writeTestImage(t, filepath.Join(dir, "invalid.png"), []byte("not an image"))
	for _, path := range []string{invalid, filepath.Join(dir, "missing.png")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, err := Build(Spec{Blocks: []Block{{Type: "image", Path: path}}}, "")
			if err == nil {
				t.Fatalf("Build unexpectedly accepted %q", path)
			}
		})
	}
}
