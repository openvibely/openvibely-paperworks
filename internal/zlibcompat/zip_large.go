package zlibcompat

import "fmt"

// Altered Go adaptation of zlib 1.2.12 deflate_slow. Copyright (C)
// 1995-2022 Jean-loup Gailly and Mark Adler; see LICENSE.
// DeflateZIPLevel6 compresses complete ZIP members with level 6/memLevel 8.
// Absolute match positions retain history without moving memory. Block storage
// eligibility follows the reference sliding window. Input is bounded at 64 MiB;
// arbitrary streaming calls, dictionaries and other settings are not supported.
func DeflateZIPLevel6(input []byte) ([]byte, error) {
	if len(input) > 64<<20 {
		return nil, fmt.Errorf("zlibcompat: input exceeds %d-byte tokenization bound", 64<<20)
	}
	// zlib permits speculative comparisons past lookahead in its initialized
	// window, then caps the result to lookahead. Preserve those zero guard bytes.
	window := make([]byte, len(input)+258)
	copy(window, input)
	var head, prev [32768]int
	insert := func(pos int) int {
		h := ((int(window[pos]) << 10) ^ (int(window[pos+1]) << 5) ^ int(window[pos+2])) & 32767
		old := head[h]
		prev[pos&32767], head[h] = old, pos
		return old
	}
	// Position zero is zlib's NIL sentinel, intentionally excluded as a match.
	pos, matchLength, matchStart := 0, 2, 0
	pending := false
	tokens := make([]Token, 0, 16383)
	writer := &bitWriter{}
	blockStart, windowBase := 0, 0
	flush := func(end int, final bool) {
		var raw []byte
		if blockStart >= windowBase {
			raw = input[blockStart:end]
		}
		writeTokenBlock(writer, tokens, raw, final)
		tokens = tokens[:0]
		blockStart = end
	}
	for pos < len(input) {
		if pos >= windowBase+65274 {
			windowBase += 32768
		}
		lookahead := len(input) - pos
		candidate := 0
		if lookahead >= 3 {
			candidate = insert(pos)
			// Sliding maps the new window origin to zlib's NIL sentinel.
			if candidate <= windowBase {
				candidate = 0
			}
		}
		previousLength, previousStart := matchLength, matchStart
		matchLength = 2
		if candidate != 0 && previousLength < 16 && pos-candidate <= 32506 {
			best, nice, chain := previousLength, min(128, lookahead), 128
			if previousLength >= 8 {
				chain >>= 2
			}
			limit := max(0, pos-32506)
			for {
				// Same cheap rejection checks as zlib, with a bounded byte
				// loop in place of its unrolled word-sized comparisons.
				if window[candidate+best] == window[pos+best] &&
					window[candidate+best-1] == window[pos+best-1] &&
					window[candidate] == window[pos] && window[candidate+1] == window[pos+1] {
					n := 2
					for n < 258 && window[candidate+n] == window[pos+n] {
						n++
					}
					if n > best {
						best, matchStart = n, candidate
						if n >= nice {
							break
						}
					}
				}
				candidate = prev[candidate&32767]
				chain--
				if candidate <= limit || chain == 0 {
					break
				}
			}
			matchLength = min(best, lookahead)
			if matchLength == 3 && pos-matchStart > 4096 {
				matchLength = 2
			}
		}
		if previousLength >= 3 && matchLength <= previousLength {
			tokens = append(tokens, Token{uint16(previousLength), uint16(pos - 1 - previousStart)})
			for n := previousLength - 2; n > 0; n-- {
				pos++
				if pos <= len(input)-3 {
					insert(pos)
				}
			}
			pending, matchLength = false, 2
			pos++
			if len(tokens) == 16383 {
				flush(pos, false)
			}
		} else if pending {
			tokens = append(tokens, Token{Value: uint16(window[pos-1])})
			if len(tokens) == 16383 {
				flush(pos, false)
			}
			pos++
		} else {
			pending = true
			pos++
		}
	}
	if pending {
		tokens = append(tokens, Token{Value: uint16(window[pos-1])})
	}
	flush(pos, true)
	writer.align()
	return writer.data, nil
}
