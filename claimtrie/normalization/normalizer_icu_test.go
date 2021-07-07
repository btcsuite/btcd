//go:build use_icu_normalization
// +build use_icu_normalization

package normalization

import (
	"bytes"
	"encoding/hex"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestNormalizationICU(t *testing.T) {
	testNormalization(t, normalizeICU)
}

func BenchmarkNormalizeICU(b *testing.B) {
	benchmarkNormalize(b, normalizeICU)
}

var testStrings = []string{
	"Les-Masques-Blancs-Die-Dead-place-Sathonay-28-Août",
	"Bez-komentu-výbuch-z-vnútra,-radšej-pozri-video...-",
	"၂-နစ်အကြာမှာ",
	"ငရဲပြည်မှ-6",
	"@happyvision",
	"ကမ္ဘာပျက်ကိန်း-9",
	"ဝိညာဉ်နား၊-3",
	"un-amore-nuovo-o-un-ritorno-cosa-mi-dona",
	"è-innamorato-di-me-anche-se-non-lo-dice",
	"ပြင်ဆင်ပါ-no.1",
	"ပြင်ဆင်ပါ-no.4",
	"ပြင်ဆင်ပါ-no.2",
	"ပြင်ဆင်ပါ-no.3",
	"ငရဲပြည်မှ-5",
	"ပြင်ဆင်ပါ-no.6",
	"ပြင်ဆင်ပါ-no.5",
	"ပြင်ဆင်ပါ-no.7",
	"ပြင်ဆင်ပါ-no.8",
	"အချိန်-2",
	"ဝိညာဉ်နား၊-4",
	"ပြင်ဆင်ပါ-no.-13",
	"ပြင်ဆင်ပါ-no.15",
	"ပြင်ဆင်ပါ-9",
	"schilddrüsenhormonsubstitution-nach",
	"Linxextremismus-JPzuG_UBtEg",
	"Ꮖ-Ꮩ-Ꭺ-N--------Ꭺ-N-Ꮹ-Ꭼ-Ꮮ-Ꭺ-on-Instagram_-“Our-next-destination-is-East-and-Southeast-Asia--selfie--asia”",
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
}

func TestBlock760150_1020105(t *testing.T) {
	test, _ := hex.DecodeString("43efbfbd")
	assert.True(t, utf8.Valid(test))
	a := normalizeGo(test)
	b := normalizeICU(test)
	assert.Equal(t, a, b)

	for i, s := range testStrings {
		a = normalizeGo([]byte(s))
		b = normalizeICU([]byte(s))
		assert.Equal(t, a, b, "%d: %s != %s", i, string(a), string(b))
		// t.Logf("%s -> %s", s, string(b))
	}
}

func TestBlock1085612(t *testing.T) {
	s, err := hex.DecodeString("6eccb7cd9dcc92cd90cc86cc80cc80cd91cd9dcd8acd80cd92cc94cc85cc8fccbdcda0ccbdcd80cda0cd84cc94cc8ccc9acd84cc94cd9bcda0cca7cc99ccaccd99cca9cca7")
	assert.NoError(t, err)
	a := normalizeICU(s)
	b := normalizeGo(s)
	assert.Equal(t, a, b, "%s != %s, %v", string(a), string(b), bytes.Equal(b, s))
}
