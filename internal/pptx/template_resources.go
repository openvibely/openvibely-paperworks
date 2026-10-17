package pptx

import "embed"

// These are reusable default package resources, not generated slide content.
// See assets/default/README.md for the pinned source and extraction contract.
//
//go:embed assets/default/docProps/* assets/default/ppt/*.xml assets/default/ppt/printerSettings/*
var defaultResources embed.FS

var templateResources = []struct{ name, contentType string }{
	{"docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml"},
	{"ppt/presProps.xml", "application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"},
	{"ppt/viewProps.xml", "application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml"},
	{"ppt/printerSettings/printerSettings1.bin", "application/vnd.openxmlformats-officedocument.presentationml.printerSettings"},
	{"docProps/thumbnail.jpeg", "image/jpeg"},
}

func (b *builder) addTemplateResources() error {
	for _, part := range templateResources {
		data, err := defaultResources.ReadFile("assets/default/" + part.name)
		if err != nil {
			return err
		}
		b.add(part.name, part.contentType, string(data))
	}
	return nil
}
