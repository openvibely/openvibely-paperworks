package zlibcompat

// Altered Go adaptation of zlib 1.2.12 trees.c (build_tree, gen_bitlen,
// scan_tree/send_tree and _tr_flush_block). Copyright (C) 1995-2022
// Jean-loup Gailly and Mark Adler. See LICENSE. No C or WASM is executed.

import "fmt"

var lengthBase = []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
var lengthExtra = []int{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
var distanceBase = []int{1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577}
var distanceExtra = []int{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}
var bitLengthOrder = []int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

type huffman struct {
	lengths, codes []int
	last           int
}

func makeCodes(lengths []int, last int) huffman {
	var counts, next [16]int
	for _, n := range lengths {
		if n > 0 {
			counts[n]++
		}
	}
	for n := 1; n <= 15; n++ {
		next[n] = (next[n-1] + counts[n-1]) << 1
	}
	codes := make([]int, len(lengths))
	for i, n := range lengths {
		if n == 0 {
			continue
		}
		v := next[n]
		next[n]++
		for j := 0; j < n; j++ {
			codes[i] = codes[i]<<1 | v&1
			v >>= 1
		}
	}
	return huffman{lengths, codes, last}
}

// Preserve zlib's heap tie-breaking (frequency, then depth, including <=),
// insertion order, and bounded-length overflow redistribution.
func buildTree(frequencies []int, maxBits int) huffman {
	elems := len(frequencies)
	freq := make([]int, 2*elems+1)
	copy(freq, frequencies)
	depth := make([]int, len(freq))
	dad := make([]int, len(freq))
	lens := make([]int, len(freq))
	heap := make([]int, 573)
	count, top, last := 0, len(heap), -1
	for i, f := range frequencies {
		if f != 0 {
			count++
			heap[count] = i
			last = i
		}
	}
	for count < 2 {
		n := 0
		if last < 2 {
			last++
			n = last
		}
		count++
		heap[count] = n
		freq[n] = 1
	}
	smaller := func(a, b int) bool { return freq[a] < freq[b] || freq[a] == freq[b] && depth[a] <= depth[b] }
	down := func(k int) {
		v := heap[k]
		for j := k * 2; j <= count; j = k * 2 {
			if j < count && smaller(heap[j+1], heap[j]) {
				j++
			}
			if smaller(v, heap[j]) {
				break
			}
			heap[k] = heap[j]
			k = j
		}
		heap[k] = v
	}
	for i := count / 2; i >= 1; i-- {
		down(i)
	}
	for node := elems; ; node++ {
		n := heap[1]
		heap[1] = heap[count]
		count--
		down(1)
		m := heap[1]
		top--
		heap[top] = n
		top--
		heap[top] = m
		freq[node] = freq[n] + freq[m]
		depth[node] = max(depth[n], depth[m]) + 1
		dad[n] = node
		dad[m] = node
		heap[1] = node
		down(1)
		if count < 2 {
			break
		}
	}
	top--
	heap[top] = heap[1]
	var counts [16]int
	overflow := 0
	for h := top + 1; h < len(heap); h++ {
		n := heap[h]
		bits := lens[dad[n]] + 1
		if bits > maxBits {
			bits = maxBits
			overflow++
		}
		lens[n] = bits
		if n <= last {
			counts[bits]++
		}
	}
	if overflow > 0 {
		for overflow > 0 {
			bits := maxBits - 1
			for counts[bits] == 0 {
				bits--
			}
			counts[bits]--
			counts[bits+1] += 2
			counts[maxBits]--
			overflow -= 2
		}
		h := len(heap)
		for bits := maxBits; bits > 0; bits-- {
			for n := counts[bits]; n > 0; {
				h--
				m := heap[h]
				if m > last {
					continue
				}
				lens[m] = bits
				n--
			}
		}
	}
	return makeCodes(lens[:elems], last)
}

type lengthRun struct{ symbol, value, bits int }

func treeRuns(tree huffman) []lengthRun {
	out := []lengthRun{}
	previous, next, count, maximum, minimum := -1, tree.lengths[0], 0, 7, 4
	if next == 0 {
		maximum, minimum = 138, 3
	}
	for n := 0; n <= tree.last; n++ {
		current := next
		next = 65535
		if n < tree.last {
			next = tree.lengths[n+1]
		}
		count++
		if count < maximum && current == next {
			continue
		}
		if count < minimum {
			for j := 0; j < count; j++ {
				out = append(out, lengthRun{symbol: current})
			}
		} else if current != 0 {
			if current != previous {
				out = append(out, lengthRun{symbol: current})
				count--
			}
			out = append(out, lengthRun{16, count - 3, 2})
		} else if count <= 10 {
			out = append(out, lengthRun{17, count - 3, 3})
		} else {
			out = append(out, lengthRun{18, count - 11, 7})
		}
		count, previous = 0, current
		if next == 0 {
			maximum, minimum = 138, 3
		} else if current == next {
			maximum, minimum = 6, 3
		} else {
			maximum, minimum = 7, 4
		}
	}
	return out
}

type bitWriter struct {
	data    []byte
	pending uint64
	bits    uint
}

func (w *bitWriter) put(v, n int) {
	w.pending |= uint64(v) << w.bits
	w.bits += uint(n)
	for w.bits >= 8 {
		w.data = append(w.data, byte(w.pending))
		w.pending >>= 8
		w.bits -= 8
	}
}
func (w *bitWriter) align() {
	if w.bits > 0 {
		w.data = append(w.data, byte(w.pending))
		w.bits = 0
		w.pending = 0
	}
}
func (w *bitWriter) symbol(tree huffman, n int) { w.put(tree.codes[n], tree.lengths[n]) }
func codeIndex(bases []int, n int) int {
	i := len(bases) - 1
	for bases[i] > n {
		i--
	}
	return i
}

// DeflateLevel6 returns a raw, final DEFLATE stream for one complete input.
// It supports only the no-window-slide, single-token-block subset of zlib's
// level 6/memLevel 8/default strategy. Unsupported inputs fail without output;
// there is no fallback to a compressor with different byte output.
func DeflateLevel6(input []byte) ([]byte, error) {
	tokens, err := TokenizeLevel6(input)
	if err != nil {
		return nil, err
	}
	// zlib flushes when its memLevel-8 symbol buffer reaches 16383 tokens.
	if len(tokens) >= 16383 {
		return nil, fmt.Errorf("zlibcompat: multiple token blocks unsupported")
	}
	w := &bitWriter{}
	writeTokenBlock(w, tokens, input, true)
	w.align()
	return w.data, nil
}

// writeTokenBlock preserves the pinned tree construction and bit ordering.
// A nil stored slice means the block has slid out of the input window.
func writeTokenBlock(w *bitWriter, tokens []Token, input []byte, final bool) {
	last := 0
	if final {
		last = 1
	}
	lf, df := make([]int, 286), make([]int, 30)
	lf[256] = 1
	extraBits := 0
	for _, t := range tokens {
		if t.Distance == 0 {
			lf[t.Value]++
		} else {
			l := codeIndex(lengthBase, int(t.Value))
			d := codeIndex(distanceBase, int(t.Distance))
			lf[257+l]++
			df[d]++
			extraBits += lengthExtra[l] + distanceExtra[d]
		}
	}
	lit, dist := buildTree(lf, 15), buildTree(df, 15)
	fixedLens := make([]int, 288)
	for i := range fixedLens {
		switch {
		case i < 144:
			fixedLens[i] = 8
		case i < 256:
			fixedLens[i] = 9
		case i < 280:
			fixedLens[i] = 7
		default:
			fixedLens[i] = 8
		}
	}
	fixedDistLens := make([]int, 32)
	for i := range fixedDistLens {
		fixedDistLens[i] = 5
	}
	fixedLit, fixedDist := makeCodes(fixedLens, 287), makeCodes(fixedDistLens, 31)
	runs := append(treeRuns(lit), treeRuns(dist)...)
	bf := make([]int, 19)
	for _, r := range runs {
		bf[r.symbol]++
	}
	bl := buildTree(bf, 7)
	blCount := 19
	for blCount > 4 && bl.lengths[bitLengthOrder[blCount-1]] == 0 {
		blCount--
	}
	dynamicBits, staticBits := extraBits+14+3*blCount, extraBits
	for i, f := range lf {
		dynamicBits += f * lit.lengths[i]
		staticBits += f * fixedLit.lengths[i]
	}
	for i, f := range df {
		dynamicBits += f * dist.lengths[i]
		staticBits += f * fixedDist.lengths[i]
	}
	for _, r := range runs {
		dynamicBits += bl.lengths[r.symbol] + r.bits
	}
	dynamicBytes, staticBytes := (dynamicBits+10)>>3, (staticBits+10)>>3
	if input != nil && len(input)+4 <= min(dynamicBytes, staticBytes) {
		w.put(last, 3)
		w.align()
		n := len(input)
		w.put(n, 16)
		w.put(n^65535, 16)
		w.data = append(w.data, input...)
		return
	}
	if staticBytes <= dynamicBytes {
		w.put(2+last, 3)
		lit, dist = fixedLit, fixedDist
	} else {
		w.put(4+last, 3)
		w.put(lit.last-256, 5)
		w.put(dist.last, 5)
		w.put(blCount-4, 4)
		for _, i := range bitLengthOrder[:blCount] {
			w.put(bl.lengths[i], 3)
		}
		for _, r := range runs {
			w.symbol(bl, r.symbol)
			w.put(r.value, r.bits)
		}
	}
	for _, t := range tokens {
		if t.Distance == 0 {
			w.symbol(lit, int(t.Value))
		} else {
			l := codeIndex(lengthBase, int(t.Value))
			d := codeIndex(distanceBase, int(t.Distance))
			w.symbol(lit, 257+l)
			w.put(int(t.Value)-lengthBase[l], lengthExtra[l])
			w.symbol(dist, d)
			w.put(int(t.Distance)-distanceBase[d], distanceExtra[d])
		}
	}
	w.symbol(lit, 256)
}
