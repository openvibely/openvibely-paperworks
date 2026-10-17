package testsupport

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ulikunitz/xz/lzma"
)

// Reference ZipExtFile._read1 decompresses before slicing to the ZIP-declared
// length. The classic Go codec instead enforces its header size. Decode with
// an independent, growing size bound, then apply ZIP's slice and CRC outside
// the codec. Replaying on the codec's size boundary also bounds dictionary
// allocation by decoded output, even for a tiny stream with a 4-GiB dictionary.
// This adapter reads a member in memory, like the original mutation helper.
func readMutationLZMA(source io.Reader, declared uint64) ([]byte, error) {
	compressed, err := io.ReadAll(source)
	if err != nil {
		return nil, err
	}
	// The pinned codec accepts classic sizes up to a pebibyte. This remains a
	// codec limit, not a claim of complete liblzma behavior at arbitrary sizes.
	const maxSize = uint64(1 << 50)
	for size := uint64(4096); ; size *= 2 {
		r := &mutationLZMAReader{source: bytes.NewReader(compressed), size: size}
		data, err := io.ReadAll(r)
		ended := r.decoder != nil && r.decoder.EOSMarker()
		boundary := !ended && (err != nil && err.Error() == "lzma: wrong uncompressed data size" || err == nil && uint64(len(data)) >= size)
		if boundary && size < maxSize {
			continue
		}
		if err != nil {
			return nil, err
		}
		if boundary {
			return nil, fmt.Errorf("LZMA decoded size exceeds codec limit")
		}
		if uint64(len(data)) > declared {
			data = data[:int(declared)]
		}
		return data, nil
	}
}

// Reference zipfile.LZMADecompressor ignores the version bytes and reads the
// properties length before decoding one raw LZMA1 stream. The classic codec
// gets those same properties and an independent trial size, not the ZIP size.
type mutationLZMAReader struct {
	source  io.Reader
	decoder *lzma.Reader
	size    uint64
}

func (r *mutationLZMAReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.decoder == nil {
		var prefix [4]byte
		if _, err := io.ReadFull(r.source, prefix[:]); err != nil {
			return 0, mutationLZMAEOF(err)
		}
		size := int(binary.LittleEndian.Uint16(prefix[2:]))
		// Reference waits for at least one data byte after all property bytes before
		// validating them. Incomplete properties can represent an empty ZIP member.
		properties := make([]byte, size+1)
		if _, err := io.ReadFull(r.source, properties); err != nil {
			return 0, mutationLZMAEOF(err)
		}
		if size != 5 {
			return 0, fmt.Errorf("invalid ZIP LZMA properties length %d", size)
		}
		props, err := lzma.PropertiesForCode(properties[0])
		if err != nil {
			return 0, err
		}
		// liblzma's LZMA1 filter requires lc+lp <= 4; the Go codec permits more.
		if props.LC+props.LP > 4 {
			return 0, fmt.Errorf("invalid ZIP LZMA literal properties")
		}
		header := make([]byte, 13)
		copy(header, properties[:5])
		binary.LittleEndian.PutUint64(header[5:], r.size)
		r.decoder, err = (lzma.ReaderConfig{DictCap: int(lzma.MaxDictCap)}).NewReader(io.MultiReader(bytes.NewReader(header), bytes.NewReader(properties[5:]), r.source))
		if err != nil {
			return 0, mutationLZMAEOF(err)
		}
	}
	n, err := r.decoder.Read(p)
	// With a validated end marker, the classic decoder's size error means
	// only that the trial length differs. Reference's raw LZMA decoder
	// has no expected length and accepts this termination. Keep every other
	// codec error, including size errors before an end marker. The string is
	// the unexported errSize constant in the pinned xz v0.5.15 decoder.
	endedBeforeSize := err != nil && err.Error() == "lzma: wrong uncompressed data size" && r.decoder.EOSMarker()
	if err == io.ErrUnexpectedEOF || endedBeforeSize {
		// The decoder marks input exhaustion before returning bytes buffered in
		// its dictionary. Drain those bytes before exposing ZIP's EOF/CRC check.
		m, e := r.decoder.Read(p[n:])
		n += m
		if e != nil && e != io.EOF {
			return n, e
		}
		if n == len(p) {
			return n, nil
		}
		return n, io.EOF
	}
	return n, err
}

func mutationLZMAEOF(err error) error {
	if err == io.ErrUnexpectedEOF {
		return io.EOF
	}
	return err
}
