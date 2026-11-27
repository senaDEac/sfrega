package native

import (
	"image"
	"path/filepath"
	"sort"
	"strings"

	"sfrega/internal/player"
)

// plMeta deriva, a partir da lista de faixas de uma janela, os mesmos índices
// que o player mantém para a lista ativa (gêneros, artistas, álbuns, pastas).
// Os tags vêm do cache compartilhado por path, então as três janelas podem
// conviver sem recalcular nada do que já existe.
type plMeta struct {
	p      *player.Player
	tracks []string

	genres  []string
	artists []string
	albums  []string
	folders []string

	genreTracks  map[string][]int
	artistTracks map[string][]int
	albumTracks  map[string][]int
	folderTracks map[string][]int

	genreFirst  map[string]string
	artistFirst map[string]string
	albumFirst  map[string]string
	folderFirst map[string]string
}

// rebuildMeta recalcula os índices a partir de pl.tracks (chamado quando a
// lista da janela muda).
func (pl *plw) rebuildMeta() {
	m := pl.meta
	if m.p == nil {
		m.p = pl.ui.p
	}
	m.tracks = pl.tracks
	m.genres, m.artists, m.albums, m.folders = nil, nil, nil, nil
	m.genreTracks = map[string][]int{}
	m.artistTracks = map[string][]int{}
	m.albumTracks = map[string][]int{}
	m.folderTracks = map[string][]int{}
	m.genreFirst = map[string]string{}
	m.artistFirst = map[string]string{}
	m.albumFirst = map[string]string{}
	m.folderFirst = map[string]string{}

	seenG, seenA, seenAl, seenF := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, path := range pl.tracks {
		artist, _, album, genre := m.p.TagsForPath(path)
		if genre != "" {
			m.genreTracks[genre] = append(m.genreTracks[genre], i)
			if !seenG[genre] {
				seenG[genre] = true
				m.genreFirst[genre] = path
				m.genres = append(m.genres, genre)
			}
		}
		if artist != "" {
			m.artistTracks[artist] = append(m.artistTracks[artist], i)
			if !seenA[artist] {
				seenA[artist] = true
				m.artistFirst[artist] = path
				m.artists = append(m.artists, artist)
			}
		}
		if album != "" {
			m.albumTracks[album] = append(m.albumTracks[album], i)
			if !seenAl[album] {
				seenAl[album] = true
				m.albumFirst[album] = path
				m.albums = append(m.albums, album)
			}
		}
		dir := filepath.Dir(path)
		m.folderTracks[dir] = append(m.folderTracks[dir], i)
		if !seenF[dir] {
			seenF[dir] = true
			m.folderFirst[dir] = path
			m.folders = append(m.folders, dir)
		}
	}
	sort.Slice(m.genres, func(a, b int) bool {
		return strings.ToLower(m.genres[a]) < strings.ToLower(m.genres[b])
	})
	sort.Slice(m.artists, func(a, b int) bool {
		return strings.ToLower(m.artists[a]) < strings.ToLower(m.artists[b])
	})
	sort.Slice(m.albums, func(a, b int) bool {
		return strings.ToLower(m.albums[a]) < strings.ToLower(m.albums[b])
	})
	sort.Slice(m.folders, func(a, b int) bool {
		return strings.ToLower(filepath.Base(m.folders[a])) < strings.ToLower(filepath.Base(m.folders[b]))
	})
	pl.dirty = false
}

// ---- gêneros ----

func (m *plMeta) Genres() []string { return m.genres }

func (m *plMeta) TracksForGenre(g string) []int {
	if g == "" {
		out := make([]int, len(m.tracks))
		for i := range out {
			out[i] = i
		}
		return out
	}
	return append([]int(nil), m.genreTracks[g]...)
}

func (m *plMeta) GenreTrackCount(g string) int { return len(m.genreTracks[g]) }

func (m *plMeta) GenreCover(g string) image.Image { return m.cover(m.genreFirst[g]) }

// ---- artistas ----

func (m *plMeta) Artists() []string { return m.artists }

func (m *plMeta) TracksForArtist(a string) []int {
	if a == "" {
		out := make([]int, len(m.tracks))
		for i := range out {
			out[i] = i
		}
		return out
	}
	return append([]int(nil), m.artistTracks[a]...)
}

func (m *plMeta) ArtistTrackCount(a string) int { return len(m.artistTracks[a]) }

func (m *plMeta) ArtistCover(a string) image.Image { return m.cover(m.artistFirst[a]) }

// ---- álbuns ----

func (m *plMeta) Albums() []string { return m.albums }

func (m *plMeta) AlbumTracks(a string) []int {
	return append([]int(nil), m.albumTracks[a]...)
}

func (m *plMeta) AlbumTrackCount(a string) int { return len(m.albumTracks[a]) }

func (m *plMeta) AlbumCover(a string) image.Image { return m.cover(m.albumFirst[a]) }

// ---- pastas ----

func (m *plMeta) Folders() []string { return m.folders }

func (m *plMeta) FolderTracks(dir string) []int {
	return append([]int(nil), m.folderTracks[dir]...)
}

func (m *plMeta) FolderTrackCount(dir string) int { return len(m.folderTracks[dir]) }

func (m *plMeta) FolderCover(dir string) image.Image { return m.cover(m.folderFirst[dir]) }

func (m *plMeta) FolderName(dir string) string { return filepath.Base(dir) }

// ---- faixas por índice (lista desta janela) ----

// TrackAt devolve o caminho da faixa i da lista desta janela.
func (m *plMeta) TrackAt(i int) string {
	if i < 0 || i >= len(m.tracks) {
		return ""
	}
	return m.tracks[i]
}

func (m *plMeta) TrackTitle(i int) (string, bool) {
	return m.p.DisplayNameForPath(m.TrackAt(i))
}

func (m *plMeta) TrackParts(i int) (artist, title string, hasMeta bool) {
	path := m.TrackAt(i)
	if path == "" {
		return "", "", false
	}
	a, t, _, _ := m.p.TagsForPath(path)
	return a, t, t != "" || a != ""
}

func (m *plMeta) TrackGenre(i int) string {
	_, _, _, g := m.p.TagsForPath(m.TrackAt(i))
	return g
}

func (m *plMeta) TrackAlbum(i int) string {
	_, _, al, _ := m.p.TagsForPath(m.TrackAt(i))
	return al
}

func (m *plMeta) TrackCover(i int) image.Image { return m.cover(m.TrackAt(i)) }

// TrackMeta repassa ao player os metadados persistidos (rating/hashtags),
// que são por caminho e valem para qualquer janela.
func (m *plMeta) TrackMeta(path string) *player.TrackMeta {
	return m.p.TrackMeta(path)
}

func (m *plMeta) cover(path string) image.Image {
	if m.p == nil {
		return nil
	}
	return m.p.CoverForPath(path)
}
