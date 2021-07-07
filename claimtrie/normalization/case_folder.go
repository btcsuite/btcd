package normalization

import (
	"bytes"
	_ "embed"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

//go:embed CaseFolding_v11.txt
var v11 string

var foldMap map[rune][]rune

func init() {
	foldMap = map[rune][]rune{}
	r, _ := regexp.Compile(`([[:xdigit:]]+?); (.); ([[:xdigit:] ]+?);`)
	matches := r.FindAllStringSubmatch(v11, 1000000000)
	for i := range matches {
		if matches[i][2] == "C" || matches[i][2] == "F" {
			key, err := strconv.ParseUint(matches[i][1], 16, len(matches[i][1])*4)
			if err != nil {
				panic(err)
			}
			splits := strings.Split(matches[i][3], " ")
			var values []rune
			for j := range splits {
				value, err := strconv.ParseUint(splits[j], 16, len(splits[j])*4)
				if err != nil {
					panic(err)
				}
				values = append(values, rune(value))
			}
			foldMap[rune(key)] = values
		}
	}
}

func caseFold(name []byte) []byte {
	var b bytes.Buffer
	b.Grow(len(name))
	for i := 0; i < len(name); {
		r, w := utf8.DecodeRune(name[i:])
		if r == utf8.RuneError && w < 2 {
			// HACK: their RuneError is actually a valid character if coming from a width of 2 or more
			return name
		}
		replacements := foldMap[r]
		if len(replacements) > 0 {
			for j := range replacements {
				b.WriteRune(replacements[j])
			}
		} else {
			b.WriteRune(r)
		}
		i += w
	}
	return b.Bytes()
}
