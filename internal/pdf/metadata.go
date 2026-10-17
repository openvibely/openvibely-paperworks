package pdf

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// serializeFilled preserves the distinction between absent and empty Info.
// pdfcpu synthesizes Info for pre-2.0 PDFs even with PreserveInfoDict set.
// For absent Info only, emit a classic xref and remove the generated reference
// from its final trailer. The trailer follows every indexed object and the xref
// table, so removing this entry changes neither object offsets nor startxref.
// This is output construction, not normalization of comparison evidence.
func serializeFilled(ctx context.Context, c *model.Context, withoutInfo bool) ([]byte, error) {
	c.PreserveInfoDict = true
	if !withoutInfo {
		return Serialize(ctx, c)
	}
	if c.Info != nil {
		return nil, fmt.Errorf("cannot omit existing document information")
	}
	var err error
	c.Info, err = c.IndRefForNewObject(types.NewDict())
	if err != nil {
		return nil, err
	}
	c.WriteObjectStream = false
	c.WriteXRefStream = false
	data, err := Serialize(ctx, c)
	if err != nil {
		return nil, err
	}
	return omitGeneratedInfo(data, *c.Info)
}

func omitGeneratedInfo(data []byte, info types.IndirectRef) ([]byte, error) {
	// Only inspect the final writer-generated trailer, never page or string data.
	end := bytes.LastIndex(data, []byte("startxref"))
	if end < 0 {
		return nil, fmt.Errorf("missing startxref after PDF write")
	}
	start := bytes.LastIndex(data[:end], []byte("trailer"))
	if start < 0 {
		return nil, fmt.Errorf("missing classic trailer after PDF write")
	}
	trailer := data[start:end]
	entry := []byte(fmt.Sprintf("/Info %d %d R", info.ObjectNumber.Value(), info.GenerationNumber.Value()))
	if bytes.Count(trailer, entry) != 1 {
		return nil, fmt.Errorf("unexpected generated Info trailer entry")
	}
	at := bytes.Index(trailer, entry) + start
	following := at + len(entry)
	if following >= end || !bytes.ContainsRune([]byte("/ >\r\n\t"), rune(data[following])) {
		return nil, fmt.Errorf("invalid generated Info reference boundary")
	}
	return append(data[:at], data[following:]...), nil
}
