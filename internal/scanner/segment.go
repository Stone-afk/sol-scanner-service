package scanner

// Segments splits the inclusive [left, right] range into contiguous segments
// of at most slotsPerWorker slots. Every segment except the last has length
// slotsPerWorker; the last may be shorter. At least one segment is returned
// when left <= right.
func Segments(left, right, slotsPerWorker uint64) [][2]uint64 {
	if slotsPerWorker == 0 || left > right {
		return nil
	}
	var out [][2]uint64
	for start := left; start <= right; {
		end := start + slotsPerWorker - 1
		if end > right || end < start { // overflow or past right
			end = right
		}
		out = append(out, [2]uint64{start, end})
		if end == right {
			break
		}
		start = end + 1
	}
	return out
}
