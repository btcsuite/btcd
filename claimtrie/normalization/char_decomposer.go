package normalization

import (
	"bufio"
	_ "embed"
	"strconv"
	"strings"
	"unicode/utf8"
)

//go:embed NFC_v11.txt
var decompositions string // the data file that came from ICU 63.2

var nfdMap map[rune][]rune
var nfdOrder map[rune]int32

func init() {
	nfdMap = map[rune][]rune{}
	nfdOrder = map[rune]int32{}
	scanner := bufio.NewScanner(strings.NewReader(decompositions))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) <= 0 || line[0] == '#' || line[0] == '*' {
			continue
		}
		if strings.ContainsAny(line, ":") {
			// it's a ordering def:
			addOrdering(line)
			continue
		}
		splits := strings.Split(line, "=")
		if len(splits) <= 1 {
			splits = strings.Split(line, ">")
			if len(splits) <= 1 {
				continue
			}
		}
		key, err := strconv.ParseUint(splits[0], 16, len(splits[0])*4)
		if err != nil {
			panic(err)
		}
		splits = strings.Split(splits[1], " ")
		values := make([]rune, 0, len(splits))
		for j := range splits {
			value, err := strconv.ParseUint(splits[j], 16, len(splits[j])*4)
			if err != nil {
				panic(err)
			}
			existing := nfdMap[rune(value)]
			if len(existing) > 0 {
				values = append(values, existing...)
			} else {
				values = append(values, rune(value))
			}
		}
		nfdMap[rune(key)] = values
	}

	// run one more expansion pass to catch stragglers
	for key, values := range nfdMap {
		for i, value := range values {
			other := nfdMap[value]
			if len(other) > 0 {
				newValues := make([]rune, len(values)+len(other)-1)
				copy(newValues, values[:i])
				copy(newValues[i:i+len(other)], other)
				copy(newValues[i+len(other):], values[i+1:])
				nfdMap[key] = newValues
			}
		}
	}

	// assert no more expansions are necessary:
	for _, values := range nfdMap {
		for _, value := range values {
			other := nfdMap[value]
			if len(other) > 0 {
				panic("Failed in NFD expansion")
			}
		}
	}
}

func addOrdering(line string) {
	splits := strings.Split(line, ":")
	ranges := strings.Split(splits[0], "..")

	value, err := strconv.ParseUint(splits[1], 16, len(splits[1])*4)
	if err != nil {
		panic(err)
	}

	start, err := strconv.ParseUint(ranges[0], 16, len(ranges[0])*4)
	if err != nil {
		panic(err)
	}
	end := start
	if len(ranges) > 1 {
		end, err = strconv.ParseUint(ranges[1], 16, len(ranges[0])*4)
		if err != nil {
			panic(err)
		}
	}
	for i := start; i <= end; i++ {
		nfdOrder[rune(i)] = int32(value)
	}
}

func decompose(name []byte) []byte {
	// see https://unicode.org/reports/tr15/ section 1.3
	runes := make([]rune, 0, len(name)) // we typically use ascii don't increase the length
	for i := 0; i < len(name); {
		r, w := utf8.DecodeRune(name[i:])
		if r == utf8.RuneError && w < 2 {
			// HACK: their RuneError is actually a valid character if coming from a width of 2 or more
			return name
		}
		replacements := nfdMap[r]
		if len(replacements) > 0 {
			runes = append(runes, replacements...)
		} else {
			hanguls := decomposeHangul(r)
			if len(hanguls) > 0 {
				runes = append(runes, hanguls...)
			} else {
				runes = append(runes, r)
			}
		}
		i += w
	}
	repairOrdering(runes)
	return []byte(string(runes))
}

func decomposeHangul(s rune) []rune {
	// see https://www.unicode.org/versions/Unicode11.0.0/ch03.pdf

	const SBase int32 = 0xAC00
	const LBase int32 = 0x1100
	const VBase int32 = 0x1161
	const TBase int32 = 0x11A7
	const LCount int32 = 19
	const VCount int32 = 21
	const TCount int32 = 28
	const NCount = VCount * TCount // 588
	const SCount = LCount * NCount // 11172

	SIndex := s - SBase
	if SIndex < 0 || SIndex >= SCount {
		return nil
	}
	L := LBase + SIndex/NCount
	V := VBase + (SIndex%NCount)/TCount
	T := TBase + SIndex%TCount
	result := []rune{L, V}
	if T != TBase {
		result = append(result, T)
	}
	return result
}

func repairOrdering(runes []rune) {
	for i := 1; i < len(runes); i++ {
		a := runes[i-1]
		b := runes[i]
		oa := nfdOrder[a]
		ob := nfdOrder[b]
		if oa > ob && ob > 0 {
			runes[i-1], runes[i] = b, a
			if i >= 2 {
				i -= 2
			} else {
				i = 0
			}
		}
	}
}
