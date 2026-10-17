package zlibcompat

// Altered Go adaptation of zlib-ng 2.2.5 deflate_medium.c, fill_window,
// insert_string and longest_match. Copyright (C) 1995-2024 Jean-loup Gailly
// and Mark Adler; portions Intel Corporation (2013), 12Sided Technology (2016),
// and Konstantin Nosov (2014-2021). See README.md and LICENSE-NG.
import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/adler32"
)

// ZlibNGRows reproduces the level-6/default-strategy/memLevel-8 row input
// schedule. The caller supplies complete filtered rows; this is not an io.Writer
// with arbitrary flush, dictionary or compression-setting support.
func ZlibNGRows(input []byte, rowBytes int) ([]byte, error) {
	return zlibNGRows(context.Background(), input, rowBytes, 0)
}

// ZlibNGPNG uses libspng 0.7.4's 8192-byte output buffers as well as row input.
func ZlibNGPNG(ctx context.Context, input []byte, rowBytes int) ([]byte, error) {
	return zlibNGRows(ctx, input, rowBytes, 8192)
}

// ZlibNGPage uses Pillow's level-6 filtered strategy and memLevel 9.
// At this level zlib-ng uses deflate_medium, which does not branch on strategy.
func ZlibNGPage(ctx context.Context, input []byte, rowBytes int) ([]byte, error) {
	return ngRows(ctx, input, rowBytes, 65536, 32767)
}
func zlibNGRows(ctx context.Context, input []byte, rowBytes, outputSize int) ([]byte, error) {
	return ngRows(ctx, input, rowBytes, outputSize, 16383)
}
func ngRows(ctx context.Context, input []byte, rowBytes, outputSize, tokenLimit int) ([]byte, error) {
	if rowBytes <= 0 || len(input)%rowBytes != 0 || len(input) > 256<<20 {
		return nil, fmt.Errorf("zlibcompat: invalid row size or input exceeds 256 MiB")
	}
	s := &ngState{window: make([]byte, 65536+258), tokens: make([]Token, 0, tokenLimit), tokenLimit: tokenLimit, writer: &bitWriter{}, outputSize: outputSize}
	for i := 0; i < len(input); i += rowBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.run(input[i:i+rowBytes], false)
	}
	s.run(nil, true)
	s.writer.align()
	out := append([]byte{0x78, 0x9c}, s.writer.data...)
	out = binary.BigEndian.AppendUint32(out, adler32.Checksum(input))
	return out, nil
}

type ngMatch struct{ start, length, pos, original int }
type ngState struct {
	outputSize, tokenLimit                          int
	window                                          []byte
	head                                            [65536]int
	prev                                            [32768]int
	pos, look, matchStart, insert, high, blockStart int
	tokens                                          []Token
	writer                                          *bitWriter
}

func (s *ngState) put(pos int) int {
	h := (binary.LittleEndian.Uint32(s.window[pos:]) * 2654435761) >> 16
	old := s.head[h]
	if old != pos {
		s.prev[pos&32767] = old
		s.head[h] = pos
	}
	return old
}
func (s *ngState) fill(in *[]byte) {
	for {
		more := 65536 - s.look - s.pos
		if s.pos >= 65274 {
			copy(s.window, s.window[32768:65536])
			s.matchStart = max(0, s.matchStart-32768)
			s.pos -= 32768
			s.blockStart -= 32768
			s.insert = min(s.insert, s.pos)
			for i, v := range s.head {
				s.head[i] = max(0, v-32768)
			}
			for i, v := range s.prev {
				s.prev[i] = max(0, v-32768)
			}
			more += 32768
		}
		if len(*in) == 0 {
			break
		}
		n := min(more, len(*in))
		copy(s.window[s.pos+s.look:], (*in)[:n])
		*in = (*in)[n:]
		s.look += n
		if s.look+s.insert >= 3 {
			start := s.pos - s.insert
			if start >= 1 {
				s.put(start - 1)
			}
			count := s.insert
			if s.look == 1 {
				count--
			}
			for i := 0; i < count; i++ {
				s.put(start + i)
			}
			s.insert -= count
		}
		if s.look >= 262 || len(*in) == 0 {
			break
		}
	}
	curr := s.pos + s.look
	if s.high < 65536 {
		if s.high < curr {
			n := min(258, 65536-curr)
			clear(s.window[curr : curr+n])
			s.high = curr + n
		} else if s.high < curr+258 {
			n := min(curr+258-s.high, 65536-s.high)
			clear(s.window[s.high : s.high+n])
			s.high += n
		}
	}
}
func (s *ngState) longest(candidate int) int {
	best, chain := 2, 128
	limit := max(0, s.pos-32506)
	for candidate < s.pos {
		// The fixed-width reference rejection compares all prefix/end bytes.
		size, offset := 2, best-1
		if best >= 4 {
			size = 4
			offset -= 2
		}
		if best >= 8 {
			size = 8
			offset -= 4
		}
		same := true
		for i := 0; i < size; i++ {
			if s.window[candidate+i] != s.window[s.pos+i] || s.window[candidate+offset+i] != s.window[s.pos+offset+i] {
				same = false
				break
			}
		}
		if same {
			n := 2
			for n < 258 && s.window[candidate+n] == s.window[s.pos+n] {
				n++
			}
			if n > best {
				s.matchStart = candidate
				if n > s.look {
					return s.look
				}
				best = n
				if best >= 128 {
					return best
				}
			}
		}
		chain--
		candidate = s.prev[candidate&32767]
		if chain == 0 || candidate <= limit {
			break
		}
	}
	return best
}
func (s *ngState) find() ngMatch {
	m := ngMatch{pos: s.pos, original: s.pos, length: 1}
	head := 0
	if s.look >= 4 {
		head = s.put(s.pos)
	}
	if head != 0 && s.pos-head > 0 && s.pos-head <= 32506 {
		m.length = s.longest(head)
		m.start = s.matchStart
		if m.length < 4 || m.start >= m.pos {
			m.length = 1
		}
	}
	return m
}
func (s *ngState) insertMatch(m ngMatch) {
	if s.look <= m.length+4 {
		return
	}
	if m.length < 4 {
		m.pos++
		m.length--
		if m.length > 0 && m.pos >= m.original {
			for i := 0; i < m.length; i++ {
				s.put(m.pos + i)
			}
		}
		return
	}
	// Level six permits insertion for all DEFLATE match lengths (<=16*16).
	if m.length <= 256 && s.look >= 4 {
		m.pos++
		m.length--
		start := max(m.pos, m.original)
		for i := start; i < m.pos+m.length; i++ {
			s.put(i)
		}
	} else {
		m.pos += m.length
		if m.pos >= 1 {
			s.put(m.pos - 1)
		}
	}
}
func (s *ngState) fizzle(c, n *ngMatch) {
	if c.length <= 1 || c.length > 1+n.start || c.length > 1+n.pos {
		return
	}
	if s.window[n.start-c.length+1] != s.window[n.pos-c.length+1] {
		return
	}
	a, b := *c, *n
	limit := max(0, n.pos-32506)
	changed := false
	for s.window[b.start-1] == s.window[b.pos-1] {
		if a.length < 1 || b.pos <= limit || b.length >= 256 || b.start <= 1 {
			break
		}
		b.pos--
		b.start--
		b.length++
		a.length--
		changed = true
	}
	if changed && a.length <= 1 && b.length != 2 {
		b.original++
		*c = a
		*n = b
	}
}
func (s *ngState) flush(final bool) {
	var raw []byte
	if s.blockStart >= 0 {
		raw = s.window[s.blockStart:s.pos]
	}
	writeTokenBlock(s.writer, s.tokens, raw, final)
	s.tokens = s.tokens[:0]
	s.blockStart = s.pos
}
func (s *ngState) run(in []byte, final bool) {
	next := ngMatch{}
	for {
		if s.look < 262 {
			s.fill(&in)
			if s.look < 262 && !final {
				return
			}
			if s.look == 0 {
				break
			}
			next.length = 0
		}
		current := next
		next = ngMatch{}
		if current.length == 0 {
			current = s.find()
		}
		s.insertMatch(current)
		if s.look > 262 && current.pos+current.length < 65274 {
			s.pos = current.pos + current.length
			next = s.find()
			if next.length >= 4 {
				s.fizzle(&current, &next)
			}
			s.pos = current.pos
		}
		if current.length < 4 {
			for i := 0; i < current.length; i++ {
				s.tokens = append(s.tokens, Token{Value: uint16(s.window[current.pos+i])})
			}
		} else {
			s.tokens = append(s.tokens, Token{uint16(current.length), uint16(current.pos - current.start)})
		}
		s.look -= current.length
		s.pos += current.length
		if len(s.tokens) >= s.tokenLimit {
			before := len(s.writer.data) + 2
			s.flush(false)
			if s.outputSize > 0 && (len(s.writer.data)+2)/s.outputSize > before/s.outputSize {
				next = ngMatch{}
			}
		}
	}
	s.insert = min(s.pos, 2)
	s.flush(true)
}
