package testsupport

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
	"time"
)

// PresentationFixtureBytes mirrors the seekable reference presentation library ZIP writer's
// member metadata for bounded presentation fixtures. Call OrderPresentationFixture
// first. Payloads use the bounded native-Go zlib-compatible encoder; unsupported
// members fail explicitly. Exact bytes require matching clocks. Production OPC writing
// retains its original metadata preservation policy.
func PresentationFixtureBytes(p *opc.Package) ([]byte, error) {
	return presentationFixtureBytes(p, time.Now)
}

func presentationFixtureBytes(p *opc.Package, clock func() time.Time) ([]byte, error) {
	return referenceFixtureBytes(p, clock, zlibcompat.DeflateLevel6)
}

// DocumentFixtureBytes uses the complete-member level-6 encoder for DOCX fixtures.
// It shares seekable metadata with newly authored DOCX; mutation policy is separate.
func DocumentFixtureBytes(p *opc.Package) ([]byte, error) {
	return referenceFixtureBytes(p, time.Now, zlibcompat.DeflateZIPLevel6)
}

func referenceFixtureBytes(p *opc.Package, clock func() time.Time, compress func([]byte) ([]byte, error)) ([]byte, error) {
	return opc.ReferenceZIPBytes(p, clock, compress)
}
