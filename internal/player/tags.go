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
	Genre  string
}

// readAudioTag extrai título/artista do arquivo de áudio.
// Suporta ID3v2 (mp3/aac), ID3v1 e Vorbis comment (flac).
func readAudioTag(path string) tagInfo {
	f, err := os.Open(path)
	if err != nil {
		return tagInfo{}
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() < 128 {
		return tagInfo{}
	}
	// espiada inicial: decide se vale ler o bloco grande
	probe := make([]byte, 16)
	if _, err := f.ReadAt(probe, 0); err != nil {
		return tagInfo{}
	}
	tail := make([]byte, 128)
	_, _ = f.ReadAt(tail, st.Size()-128)

	magic := string(probe[:3])
	if magic != "ID3" && string(probe[:4]) != "fLaC" {
		// sem tag de cabeçalho: tenta só ID3v1 na cauda
		if t := id3v1(tail); t.Title != "" || t.Artist != "" {
			return t
		}
		return tagInfo{}
	}

	// cabeçalho (ID3v2/Vorbis ficam no início) + cauda (ID3v1 nos últimos 128B)
	head := make([]byte, 256*1024)
	n, _ := f.ReadAt(head, 0)
	head = head[:n]
	head = append(head, tail...)
	if t := id3v2(head); t.Title != "" || t.Artist != "" || t.Genre != "" {
		return t
	}
	if t := id3v1(head); t.Title != "" || t.Artist != "" {
		return t
	}
	return vorbisComment(head)
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
	case 1: // UTF-16
		if len(b) > 3 && b[1] == 0xFF && b[2] == 0xFE {
			u := make([]uint16, (len(b)-3)/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(b[3+i*2:])
			}
			return strings.TrimRight(strings.TrimSpace(string(utf16.Decode(u))), "\x00")
		}
		if len(b) > 3 && b[1] == 0xFE && b[2] == 0xFF {
			u := make([]uint16, (len(b)-3)/2)
			for i := range u {
				u[i] = binary.BigEndian.Uint16(b[3+i*2:])
			}
			return strings.TrimRight(strings.TrimSpace(string(utf16.Decode(u))), "\x00")
		}
		// sem BOM: conteúdo começa em b[1]; ASCII em UTF-16 tem um byte
		// alto/baixo zero por caractere — detecta LE vs BE
		le := len(b) > 4 && b[2] == 0 && b[4] == 0 && b[1] != 0
		if le {
			u := make([]uint16, (len(b)-1)/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(b[1+i*2:])
			}
			return strings.TrimRight(strings.TrimSpace(string(utf16.Decode(u))), "\x00")
		}
		u := make([]uint16, (len(b)-1)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[1+i*2:])
		}
		return strings.TrimRight(strings.TrimSpace(string(utf16.Decode(u))), "\x00")
	case 2: // UTF-16BE
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		return strings.TrimRight(strings.TrimSpace(string(utf16.Decode(u))), "\x00")
	default: // ISO-8859-1 / UTF-8
		return strings.TrimSpace(strings.TrimRight(string(b[1:]), "\x00 "))
	}
}

func id3v2(d []byte) tagInfo {
	var t tagInfo
	var hasTitle bool // TIT2 lido (TALB é só fallback)
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
			hasTitle = true
		case "TPE1":
			t.Artist = textEnc(d[pos+10 : fend])
		case "TCON":
			t.Genre = genreName(textEnc(d[pos+10 : fend]))
		case "TALB":
			if !hasTitle {
				t.Title = textEnc(d[pos+10 : fend])
			}
		}
		pos = fend
		if hasTitle && t.Artist != "" && t.Genre != "" {
			break
		}
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
	t := tagInfo{
		Title:  clean(d[len(d)-125 : len(d)-95]),
		Artist: clean(d[len(d)-95 : len(d)-65]),
	}
	if g := int(d[len(d)-1]); g < len(id3Genres) {
		t.Genre = id3Genres[g]
	}
	return t
}

// id3Genres é a lista padrão ID3v1 estendida pelo Winamp (0-174).
var id3Genres = []string{
	"Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge",
	"Hip-Hop", "Jazz", "Metal", "New Age", "Oldies", "Other", "Pop",
	"R&B", "Rap", "Reggae", "Rock", "Techno", "Industrial", "Alternative",
	"Ska", "Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient",
	"Trip-Hop", "Vocal", "Jazz+Funk", "Fusion", "Trance", "Classical",
	"Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel", "Noise",
	"AlternRock", "Bass", "Soul", "Punk", "Space", "Meditative",
	"Instrumental Pop", "Instrumental Rock", "Ethnic", "Gothic",
	"Darkwave", "Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance",
	"Dream", "Southern Rock", "Comedy", "Cult", "Gangsta", "Top 40",
	"Christian Rap", "Pop/Funk", "Jungle", "Native American", "Cabaret",
	"New Wave", "Psychadelic", "Rave", "Showtunes", "Trailer", "Lo-Fi",
	"Tribal", "Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical",
	"Rock & Roll", "Hard Rock", "Folk", "Folk-Rock", "National Folk",
	"Swing", "Fast Fusion", "Bebob", "Latin", "Revival", "Celtic",
	"Bluegrass", "Avantgarde", "Gothic Rock", "Progressive Rock",
	"Psychedelic Rock", "Symphonic Rock", "Slow Jam", "Club", "Tango",
	"Samba", "Folklore", "Ballad", "Power Ballad", "Rhythmic Soul",
	"Freestyle", "Duet", "Punk Rock", "Drum Solo", "A capella",
	"Euro-House", "Dance Hall", "Goa", "Drum & Bass", "Club-House",
	"Hardcore", "Terror", "Indie", "BritPop", "Negerpunk", "Polsk Punk",
	"Beat", "Christian Gangsta Rap", "Heavy Metal", "Black Metal",
	"Crossover", "Contemporary Christian", "Christian Rock", "Merengue",
	"Salsa", "Thrash Metal", "Anime", "JPop", "Synthpop", "Abstract",
	"Art Rock", "Baroque", "Bhangra", "Big Beat", "Breakbeat", "Chillout",
	"Downtempo", "Dub", "EBEM", "Eclectic", "Electro", "Electroclash",
	"Emo", "Experimental", "Garage", "Global", "IDM", "Industrial Rock",
	"Jumpstyle", "Krautrock", "Leftfield", "Lounge", "Math Rock",
	"New Romantic", "Nu-Breakz", "Post-Punk", "Post-Rock", "Psytrance",
	"Shoegaze", "Space Rock", "Trop Rock", "World Music", "Neoclassical",
	"Audiobook", "Audio Theatre", "Neue Deutsche Welle", "Podcast",
	"Indie Rock", "G-Funk", "Dubstep", "Garage Rock", "Psybient",
}

// genreName normaliza referências numéricas "(17)"/"17" para o nome
// do gênero e limpa espaços.
func genreName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// "(17)" ou "17" → nome da lista
	if n, ok := parseGenreRef(s); ok {
		return n
	}
	// "(17)Rock" → "Rock Rock"? melhor: nome + resto sem o ref
	if len(s) > 3 && s[0] == '(' {
		if j := strings.IndexByte(s, ')'); j > 1 && j <= 5 {
			if n, ok := parseGenreRef(s[1:j]); ok {
				rest := strings.TrimSpace(s[j+1:])
				if rest == "" {
					return n
				}
				return n + " " + rest
			}
		}
	}
	return s
}

func parseGenreRef(s string) (string, bool) {
	s = strings.Trim(s, "() ")
	if s == "" {
		return "", false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	if n >= 0 && n < len(id3Genres) {
		return id3Genres[n], true
	}
	return "", false
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
				case "GENRE":
					t.Genre = genreName(v)
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
