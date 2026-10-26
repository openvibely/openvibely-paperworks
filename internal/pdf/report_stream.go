package pdf

import (
	"encoding/ascii85"
	"encoding/binary"
	"hash/adler32"

	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// reportStream follows ReportLab's default PDFStream filters: zlib level 6,
// then unwrapped ASCII85. Keep encoded bytes undecoded so the general PDF writer
// does not replace them with its own encoder. Deflate input is bounded at 64MiB.
func reportStream(content []byte) (types.StreamDict, error) {
	raw, err := zlibcompat.DeflateZIPLevel6(content)
	if err != nil {
		return types.StreamDict{}, err
	}
	encoded := make([]byte, ascii85.MaxEncodedLen(len(raw)+6)+2)
	n := encodeReportASCII85(raw, adler32.Checksum(content), encoded)
	encoded = append(encoded[:n], '~', '>')
	length := int64(len(encoded))
	return types.StreamDict{Dict: types.Dict{
		"Filter": types.Array{types.Name("ASCII85Decode"), types.Name("FlateDecode")},
		"Length": types.Integer(length),
	}, Raw: encoded, StreamLength: &length, FilterPipeline: []types.PDFFilter{{Name: "ASCII85Decode"}, {Name: "FlateDecode"}}}, nil
}

// encodeReportASCII85 encodes the zlib header, DEFLATE payload, and checksum as
// one logical byte sequence. It feeds aligned payload blocks directly to the
// ASCII85 encoder so the complete compressed payload never needs another copy.
func encodeReportASCII85(raw []byte, checksum uint32, dst []byte) int {
	var trailer [4]byte
	binary.BigEndian.PutUint32(trailer[:], checksum)
	if len(raw) < 2 {
		// DeflateZIPLevel6 currently always emits at least two bytes, but handle
		// a shorter payload without relying on that implementation detail.
		var z [8]byte
		z[0], z[1] = 0x78, 0x9c
		copy(z[2:], raw)
		copy(z[2+len(raw):], trailer[:])
		return ascii85.Encode(dst, z[:6+len(raw)])
	}

	var block [4]byte
	block[0], block[1] = 0x78, 0x9c
	copy(block[2:], raw[:2])
	n := ascii85.Encode(dst, block[:])

	// The two-byte zlib header leaves the DEFLATE payload offset two bytes
	// into an ASCII85 group. Encode its aligned middle directly from raw.
	payloadBytes := len(raw) - 2
	bulkBytes := payloadBytes &^ 3
	n += ascii85.Encode(dst[n:], raw[2:2+bulkBytes])

	remainder := raw[2+bulkBytes:]
	if len(remainder) == 0 {
		return n + ascii85.Encode(dst[n:], trailer[:])
	}

	// Complete the next 4-byte group with the start of the checksum, then
	// encode the remaining checksum bytes as ASCII85's final partial group.
	copy(block[:], remainder)
	copy(block[len(remainder):], trailer[:4-len(remainder)])
	n += ascii85.Encode(dst[n:], block[:])
	n += ascii85.Encode(dst[n:], trailer[4-len(remainder):])
	return n
}
