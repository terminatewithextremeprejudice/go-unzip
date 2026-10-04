package unzip

import (
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

const (
	ISO8859_5   string = "ISO-8859-5"
	UTF8        string = "UTF-8"
	Windows1251 string = "windows-1251"
	Windows1252 string = "windows-1252"
	Windows437  string = "windows-437"
	Big5        string = "big5"
)

// EncodeWindows1252 encodes utf8 string to superset of ISO-8859-1
func EncodeWindows1252(dec []byte) ([]byte, error) {
	enc := charmap.Windows1252.NewEncoder()
	out, err := enc.Bytes(dec)
	return out, err
}

func DecodeISO8859_5(enc []byte) ([]byte, error) {
	dec := charmap.ISO8859_5.NewDecoder()
	out, err := dec.Bytes(enc)
	return out, err
}

func DecodeWindows1251(enc []byte) ([]byte, error) {
	dec := charmap.Windows1251.NewDecoder()
	out, err := dec.Bytes(enc)
	return out, err
}

func DecodeWindows1252(enc []byte) ([]byte, error) {
	dec := charmap.Windows1252.NewDecoder()
	out, err := dec.Bytes(enc)
	return out, err
}

// DecodeCodePage437 decodes Codepage-437 into UTF-8 string
func DecodeWindows437(enc []byte) ([]byte, error) {
	dec := charmap.CodePage437.NewDecoder()
	out, err := dec.Bytes(enc)
	return out, err
}

// DetectUTF8 reports wheter string is a valid utf-8 string,
// and whether the string must be considered utf-8 encoding
// ie. not being compatible with Codepage-437.
func DetectUTF8(s string) (valid, require bool) {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		// Officially zip uses Codepage-437, but many readers use the system's
		// local character encoding.
		// Forbid 0x7e and 0x5c since EUC-KR and SHIFT-IJS replace those
		// characters with localized currency and overline characters.
		if r < 0x20 || r > 0x7d || r == 0x5c {
			if !utf8.ValidRune(r) || (r == utf8.RuneError && size == 1) {
				return false, false
			}
			require = true
		}
	}
	return true, require
}
