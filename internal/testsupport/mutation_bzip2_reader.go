package testsupport

import (
	"io"

	"github.com/openvibely/openvibely-paperworks/internal/testsupport/bzip2single"
)

// ZipExtFile._read1 treats exhausted compressed input as EOF even when the
// BZ2Decompressor has not consumed a complete stream footer. The surrounding
// acquisition adapter slices decoded data and checks the central ZIP CRC. Do not suppress
// structural or checksum errors from a complete BZIP2 block or stream.
type mutationBZIP2Reader struct{ io.Reader }

func newMutationBZIP2Reader(r io.Reader) io.ReadCloser {
	return io.NopCloser(mutationBZIP2Reader{bzip2single.NewReader(r)})
}

func (r mutationBZIP2Reader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}
