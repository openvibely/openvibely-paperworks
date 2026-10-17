package pptx

import (
	"embed"
	"io/fs"
	"strings"
)

// Serialized reusable reference presentation library default templates; no authored slides.
//
//go:embed all:assets/layouts
var layoutResources embed.FS

func (b *builder) addTemplateLayouts() error {
	return fs.WalkDir(layoutResources, "assets/layouts", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := layoutResources.ReadFile(name)
		if err != nil {
			return err
		}
		part := strings.TrimPrefix(name, "assets/layouts/")
		kind := ""
		if !strings.HasSuffix(part, ".rels") {
			switch {
			case strings.HasPrefix(part, "ppt/slideLayouts/"):
				kind = "application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"
			case strings.HasPrefix(part, "ppt/slideMasters/"):
				kind = "application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"
			case strings.HasPrefix(part, "ppt/notesMasters/"):
				kind = "application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml"
			case strings.HasPrefix(part, "ppt/theme/"):
				kind = "application/vnd.openxmlformats-officedocument.theme+xml"
			}
		}
		b.add(part, kind, string(data))
		return nil
	})
}
