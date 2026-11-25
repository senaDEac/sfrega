package player

import (
	"encoding/binary"
	"os"
	"strings"
	"unicode/utf16"
)

// tagInfo são os metadados de exibição de uma faixa.
type tagInfo struct {
	Title  string
	Artist string
}

// readAudioTag extrai título/artista do arquivo de áudio.
// Suporta ID3v2 (mp3/aac), ID3v1 e Vorbis comment (flac).
func readAudioTag(path string) tagInfo {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 128 {
		return tagInfo{}
	}
	if t := id3v2(data); t.Title != "" || t.Artist != "" {
		return t
	}
	if t := id3v1(data); t.Title != "" || t.Artist != "" {
		return t
	}
	return vorbisComment(data)
}

// syncSafe decodifica inteiro "syncsafe" do ID3v2.
func syncSafe(b []byte) int {
	return int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
}

func textEnc(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	switch b[0] {
	case 1: // UTF-16 com BOM
		if len(b) > 2 && b[1] == 0xFF && b[2] == 0xFE {
			u := make([]uint16, (len(b)-2)/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(b[2+i*2:])
			}
			return strings.TrimSpace(string(utf16.Decode(u)))
		}
		u := make([]uint16, (len(b)-1)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[1+i*2:])
		}
		return strings.TrimSpace(string(utf16.Decode(u)))
	case 2: // UTF-16BE
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		return strings.TrimSpace(string(utf16.Decode(u)))
	default: // ISO-8859-1 / UTF-8
		return strings.TrimSpace(strings.TrimRight(string(b[1:]), "\x00"))
	}
}

func id3v2(d []byte) tagInfo {
	var t tagInfo
	if len(d) < 10 || string(d[:3]) != "ID3" {
		return t
	}
	size := syncSafe(d[6:10])
	ver := int(d[3])<<8 | int(d[4])
	pos, end := 10, 10+size
	if end > len(d) {
		end = len(d)
	}
	for pos+10 <= end {
		id := string(d[pos : pos+4])
		if id[0] == 0 {
			break
		}
		var fsize int
		if ver >= 0x0400 { // v2.4: syncsafe
			fsize = syncSafe(d[pos+4 : pos+8])
		} else {
			fsize = int(binary.BigEndian.Uint32(d[pos+4 : pos+8]))
		}
		fend := pos + 10 + fsize
		if fend > end {
			break
		}
		switch id {
		case "TIT2":
			t.Title = textEnc(d[pos+10 : fend])
		case "TPE1":
			t.Artist = textEnc(d[pos+10 : fend])
		case "TALB":
			if t.Title == "" {
				t.Title = textEnc(d[pos+10 : fend])
			}
		}
		pos = fend
	}
	return t
}

func id3v1(d []byte) tagInfo {
	if len(d) < 128 || string(d[len(d)-128:len(d)-125]) != "TAG" {
		return tagInfo{}
	}
	clean := func(b []byte) string {
		return strings.TrimSpace(strings.TrimRight(string(b), "\x00 "))
	}
	return tagInfo{
		Title:  clean(d[len(d)-125 : len(d)-95]),
		Artist: clean(d[len(d)-95 : len(d)-65]),
	}
}

// vorbisComment lê blocos METADATA do FLAC (comentários Vorbis).
func vorbisComment(d []byte) tagInfo {
	var t tagInfo
	if len(d) < 4 || string(d[:4]) != "fLaC" {
		return t
	}
	pos := 4
	for pos+4 <= len(d) {
		h := d[pos]
		last := h&0x80 != 0
		typ := h & 0x7F
		blen := int(d[pos+1])<<16 | int(d[pos+2])<<8 | int(d[pos+3])
		pos += 4
		if pos+blen > len(d) {
			break
		}
		if typ == 4 && blen >= 8 { // VORBIS_COMMENT
			bend := pos + blen
			p := pos
			vendLen := int(binary.LittleEndian.Uint32(d[p : p+4]))
			p += 4 + vendLen
			if p+4 > bend {
				break
			}
			nc := int(binary.LittleEndian.Uint32(d[p : p+4]))
			p += 4
			for i := 0; i < nc && p+4 <= bend; i++ {
				l := int(binary.LittleEndian.Uint32(d[p : p+4]))
				p += 4
				if p+l > bend {
					break
				}
				kv := string(d[p : p+l])
				p += l
				k, v, _ := strings.Cut(kv, "=")
				switch strings.ToUpper(k) {
				case "TITLE":
					t.Title = v
				case "ARTIST":
					t.Artist = v
				}
			}
		}
		pos += blen
		if last {
			break
		}
	}
	return t
}
