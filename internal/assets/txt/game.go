package txt

import (
	"strconv"
	"strings"
)

// Lines splits a table the way the game's loaders walk it: strtok(buf, "\r") tokens
// (empty ones skipped), each line after the first starting with the '\n' of its CRLF,
// which the loaders step over. Cells may contain bare '\n' (npctext.txt does), so this
// is not the same as splitting on '\n'.
//
// mm8: 0x4ddefe (strtok with the "\r" delimiter at 0x4f9774)
func Lines(data []byte) []string {
	var out []string
	for _, t := range strings.Split(string(data), "\r") {
		if t == "" {
			continue
		}
		if len(out) > 0 {
			t = t[1:] // the loaders start each line at token + 1
		}
		out = append(out, t)
	}
	return out
}

// Cells splits a line into its TAB-separated cells, kept as they are (quotes included).
func Cells(line string) []string { return strings.Split(line, "\t") }

// StripQuotes drops the first and the last character of s when s starts with '"'.
//
// mm8: 0x451806 (Txt_StripQuotes)
func StripQuotes(s string) string {
	if len(s) > 0 && s[0] == '"' {
		return s[1 : len(s)-1+boolInt(len(s) == 1)]
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Atoi is the C runtime's atoi: leading blanks, an optional sign, then digits up to the
// first non-digit; 0 when there are none. Overflow wraps like the 32-bit original.
//
// mm8: 0x4dd7c3 (atoi -> atol 0x4dd738)
func Atoi(s string) int32 {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] >= '\t' && s[i] <= '\r') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	var v uint32
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		v = v*10 + uint32(s[i]-'0')
	}
	if neg {
		v = -v
	}
	return int32(v)
}

// Atof is the C runtime's atof for the plain decimals the tables use ("1.5"); it
// reads the longest numeric prefix.
//
// mm8: 0x4de2ee (atof)
func Atof(s string) float64 {
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	end := 0
	seenDot, seenDigit := false, false
	for end < len(s) {
		c := s[end]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
		case c == '.' && !seenDot:
			seenDot = true
		case (c == '-' || c == '+') && end == 0:
		default:
			goto done
		}
		end++
	}
done:
	if !seenDigit {
		return 0
	}
	f, _ := strconv.ParseFloat(strings.TrimRight(s[:end], "."), 64)
	return f
}
