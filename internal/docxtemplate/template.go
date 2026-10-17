// Package docxtemplate provides the pinned reference DOCX library blank package as data.
// Runtime authoring and table exchange are implemented entirely in Go.
package docxtemplate

import (
	_ "embed"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

//go:embed assets/default.docx
var blank []byte

// Open returns an independent package with the complete template relationship
// graph and all default resources. Callers may replace authored parts; no
// returned data aliases the embedded ZIP. See assets/README.md for provenance.
func Open() (*opc.Package, error) { return opc.Read(blank) }
