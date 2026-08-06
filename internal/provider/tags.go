package provider

import (
	"strconv"
	"strings"
)

func pickTag(names []string) string {
	best := ""
	for _, name := range names {
		if best == "" || newerTag(name, best) {
			best = name
		}
	}
	return best
}

func newerTag(a, b string) bool {
	av, aok := parseTagVersion(a)
	bv, bok := parseTagVersion(b)
	switch {
	case aok && bok:
		return compareDotted(av, bv) > 0
	case aok != bok:
		return aok
	default:
		return false
	}
}

func parseTagVersion(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	var parts []int
	for _, part := range strings.Split(s, ".") {
		digits := leadingDigits(part)
		if digits == "" {
			break
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			break
		}
		parts = append(parts, n)
	}
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}

func leadingDigits(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}

func compareDotted(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}
