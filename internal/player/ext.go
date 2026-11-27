package player

import "image"

// Este arquivo expõe, sem alterar o comportamento existente, o que a UI
// precisa para derivar metadados de listas que não são a ativa (Radio/Video).

// TagsForPath lê artista/título/álbum/gênero do caminho pelo cache
// compartilhado por path (mesmo cache de p.TagForPath interno).
func (p *Player) TagsForPath(path string) (artist, title, album, genre string) {
	t := p.tagForPath(path)
	return t.Artist, t.Title, t.Album, t.Genre
}

// DisplayNameForPath devolve o nome de exibição da faixa e se veio de metadados.
func (p *Player) DisplayNameForPath(path string) (string, bool) {
	t := p.tagForPath(path)
	return displayNameCached(t, path), t.Title != "" || t.Artist != ""
}

// CoverForPath devolve a capa da faixa (garante a tag no cache antes).
func (p *Player) CoverForPath(path string) image.Image {
	if path != "" {
		p.tagForPath(path)
	}
	return p.coverForPath(path)
}

// WarmTracks preenche tags e capas de uma lista que não é a ativa, sem tocar
// nos índices do player (eles são da janela que possui a lista).
func (p *Player) WarmTracks(paths []string) {
	if len(paths) == 0 {
		return
	}
	p.warmPaths(append([]string(nil), paths...), false)
}

// SetNoPersist liga/desliga a gravação do m3u e do resume: usado quando a
// lista ativa no player não é a playlist principal (ex.: Radio/Video), para
// nunca sobrescrever a playlist.m3u com a lista de outra janela.
func (p *Player) SetNoPersist(on bool) {
	p.mu.Lock()
	p.noPersist.Store(on)
	p.mu.Unlock()
}
