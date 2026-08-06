package tui

import "strings"

// fuzzyScore accepts subsequence matches and favors word boundaries and runs.
func fuzzyScore(query, candidate string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	candidate = strings.ToLower(candidate)
	if query == "" {
		return 0
	}
	pos, last, score := 0, -2, 0
	for _, want := range query {
		found := -1
		for i, got := range candidate[pos:] {
			if got == want {
				found = pos + i
				break
			}
		}
		if found < 0 {
			return -1
		}
		if found == 0 || strings.ContainsRune(" -_./", rune(candidate[found-1])) {
			score += 8
		}
		if found == last+1 {
			score += 4
		}
		score++
		last, pos = found, found+1
	}
	return score - (last - len(query))
}
