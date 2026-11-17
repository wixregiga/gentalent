package chat

func similarity(a, b string) float64 {
	left, right := []rune(a), []rune(b)
	if len(left)+len(right) == 0 {
		return 1
	}
	return 2 * float64(matchingRunes(left, right)) / float64(len(left)+len(right))
}

func matchingRunes(a, b []rune) int {
	positions := map[rune][]int{}
	for j, r := range b {
		positions[r] = append(positions[r], j)
	}
	type span struct{ alo, ahi, blo, bhi int }
	total := 0
	queue := []span{{0, len(a), 0, len(b)}}
	for len(queue) > 0 {
		s := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		i, j, size := longestMatch(a, positions, s.alo, s.ahi, s.blo, s.bhi)
		if size == 0 {
			continue
		}
		total += size
		if s.alo < i && s.blo < j {
			queue = append(queue, span{s.alo, i, s.blo, j})
		}
		if i+size < s.ahi && j+size < s.bhi {
			queue = append(queue, span{i + size, s.ahi, j + size, s.bhi})
		}
	}
	return total
}

func longestMatch(a []rune, positions map[rune][]int, alo, ahi, blo, bhi int) (int, int, int) {
	besti, bestj, bestsize := alo, blo, 0
	lengths := map[int]int{}
	for i := alo; i < ahi; i++ {
		next := map[int]int{}
		for _, j := range positions[a[i]] {
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			k := lengths[j-1] + 1
			next[j] = k
			if k > bestsize {
				besti, bestj, bestsize = i-k+1, j-k+1, k
			}
		}
		lengths = next
	}
	return besti, bestj, bestsize
}
