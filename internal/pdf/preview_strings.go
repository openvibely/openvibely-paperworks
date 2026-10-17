package pdf

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Pinned pypdf create_string_object preserves original bytes. Only its choice
// of TextStringObject vs ByteStringObject determines the serialized syntax.
// This is limited to page-copy strings; content-stream strings are untouched.
func writePreviewString(out *bytes.Buffer, raw []byte) {
	text := true
	var order binary.ByteOrder
	data := raw
	switch {
	case bytes.HasPrefix(raw, []byte{0xfe, 0xff}):
		order, data = binary.BigEndian, raw[2:]
	case bytes.HasPrefix(raw, []byte{0xff, 0xfe}):
		order, data = binary.LittleEndian, raw[2:]
	case len(raw) > 0 && raw[0] == 0:
		order = binary.BigEndian
	case len(raw) > 1 && raw[1] == 0:
		order = binary.LittleEndian
	}
	if order != nil {
		text = len(data)%2 == 0
		for i := 0; text && i < len(data); i += 2 {
			v := order.Uint16(data[i:])
			if v >= 0xd800 && v <= 0xdbff {
				if i+3 >= len(data) {
					text = false
					break
				}
				w := order.Uint16(data[i+2:])
				text = w >= 0xdc00 && w <= 0xdfff
				i += 2
			} else if v >= 0xdc00 && v <= 0xdfff {
				text = false
			}
		}
	} else {
		for _, v := range raw {
			if v == 0 || v == 22 || v == 0x7f || v == 0x9f || v == 0xad {
				text = false
				break
			}
		}
	}
	if !text {
		out.WriteByte('<')
		out.WriteString(hex.EncodeToString(raw))
		out.WriteByte('>')
		return
	}
	out.WriteByte('(')
	for _, v := range raw {
		if v == ' ' || v >= 'a' && v <= 'z' || v >= 'A' && v <= 'Z' || v >= '0' && v <= '9' {
			out.WriteByte(v)
		} else {
			fmt.Fprintf(out, "\\%03o", v)
		}
	}
	out.WriteByte(')')
}
