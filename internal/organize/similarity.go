//go:build ignore
// +build ignore

package organize

import "strings"

func stringSimilarityLevenshtein(s1, s2 string) float64 {
	s1 = strings.TrimSpace(strings.ToLower(s1))
	s2 = strings.TrimSpace(strings.ToLower(s2))
	if s1 == s2 {
		return 100
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	m, n := len(r1), len(r2)
	if m == 0 || n == 0 {
		return 0
	}

	prev := make([]int, n+1)
	curr := make([]int, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = j
	}
	for i := 1; i <= m; i++ {
		curr[0] = i
		for j := 1; j <= n; j++ {
			if r1[i-1] == r2[j-1] {
				curr[j] = prev[j-1]
			} else {
				a, b, c := prev[j], curr[j-1], prev[j-1]
				min := a
				if b < min {
					min = b
				}
				if c < min {
					min = c
				}
				curr[j] = 1 + min
			}
		}
		prev, curr = curr, prev
	}
	maxLen := m
	if n > maxLen {
		maxLen = n
	}
	return float64(maxLen-prev[n]) / float64(maxLen) * 100
}