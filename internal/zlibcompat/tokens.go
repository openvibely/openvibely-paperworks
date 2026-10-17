// Package zlibcompat contains bounded native-Go components for reproducing
// pinned zlib output. Its encoder supports bounded single-block inputs.
package zlibcompat

import "fmt"

// This is an altered Go adaptation of zlib 1.2.12 deflate.c's level-6
// deflate_slow/longest_match algorithms. Copyright (C) 1995-2022 Jean-loup
// Gailly and Mark Adler. See LICENSE and README.md for provenance and limits.

// Token describes a literal (Distance == 0, Value is the byte) or an LZ77
// match (Value is length, Distance is the backwards distance). It is not a
// Huffman code, block boundary, or encoded DEFLATE stream.
type Token struct {
	Value    uint16
	Distance uint16
}

// MaxInput bounds the implementation before zlib's window-sliding boundary.
// Streaming, alternate levels/strategies, dictionaries, and larger inputs are
// deliberately unsupported rather than silently routed to a different encoder.
const MaxInput = 32768

// TokenizeLevel6 reproduces level 6, memLevel 8, default-strategy match selection
// for one complete input. DeflateLevel6 encodes its supported single-block subset.
func TokenizeLevel6(input []byte) ([]Token, error) {
	if len(input) > MaxInput {
		return nil, fmt.Errorf("zlibcompat: input exceeds %d-byte tokenization bound", MaxInput)
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
	tokens := make([]Token, 0)
	for pos < len(input) {
		lookahead := len(input) - pos
		candidate := 0
		if lookahead >= 3 {
			candidate = insert(pos)
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
		} else if pending {
			tokens = append(tokens, Token{Value: uint16(window[pos-1])})
			pos++
		} else {
			pending = true
			pos++
		}
	}
	if pending {
		tokens = append(tokens, Token{Value: uint16(window[pos-1])})
	}
	return tokens, nil
}
