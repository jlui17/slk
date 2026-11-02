package entrylist

// plainASCIIRun maps the run of printable ASCII at the start of rest,
// one column per byte, without asking uniseg. Two printable ASCII bytes
// in a row always break into separate one-column clusters; only the
// last byte before a non-ASCII one is left for uniseg, since a
// combining mark, a variation selector or a ZWJ after it joins its
// cluster. Nothing before such a byte changes where its cluster ends,
// so the caller resumes uniseg from a fresh state. ok is false when the
// run is empty. Most of a filled row is ASCII text and padding spaces.
func plainASCIIRun(bytesMap []int, byteOffset int, rest string) ([]int, int, string, bool) {
	n := 0
	for n < len(rest) && rest[n] >= 0x20 && rest[n] < 0x7f {
		n++
	}
	if n < len(rest) && rest[n] >= 0x80 {
		n--
	}
	if n <= 0 {
		return bytesMap, byteOffset, rest, false
	}
	for k := range n {
		bytesMap = append(bytesMap, byteOffset+k)
	}
	return bytesMap, byteOffset + n, rest[n:], true
}
