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
	z := append([]byte{0x78, 0x9c}, raw...)
	z = binary.BigEndian.AppendUint32(z, adler32.Checksum(content))
	encoded := make([]byte, ascii85.MaxEncodedLen(len(z)))
	n := ascii85.Encode(encoded, z)
	encoded = append(encoded[:n], '~', '>')
	length := int64(len(encoded))
	return types.StreamDict{Dict: types.Dict{
		"Filter": types.Array{types.Name("ASCII85Decode"), types.Name("FlateDecode")},
		"Length": types.Integer(length),
	}, Raw: encoded, StreamLength: &length, FilterPipeline: []types.PDFFilter{{Name: "ASCII85Decode"}, {Name: "FlateDecode"}}}, nil
}
