// Package xmlresource loads XML authoring resources and validates their root element.
package xmlresource

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/beevik/etree"
)

// ParseError identifies errors parsing a resource after its bytes were read.
type ParseError struct {
	Err error
}

func (e *ParseError) Error() string { return e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }

// RootError reports that a resource does not have the expected root name and namespace.
type RootError struct {
	Namespace string
	LocalName string
}

func (e *RootError) Error() string {
	return fmt.Sprintf("expected {%s}%s root", e.Namespace, e.LocalName)
}

// Read resolves name relative to base when needed, reads the original bytes,
// parses them as XML, and validates the expected root element. The returned
// bytes are unchanged so callers can embed the author's original resource.
func Read(base, name, namespace, localName string) ([]byte, *etree.Document, error) {
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		return nil, nil, &ParseError{Err: err}
	}
	root := doc.Root()
	if root == nil || root.Tag != localName || root.NamespaceURI() != namespace {
		return nil, nil, &RootError{Namespace: namespace, LocalName: localName}
	}
	return raw, doc, nil
}
