# Análise — Webamp (webamp.org) → funcionalidades do Winamp 2

Fonte: https://github.com/captbaritone/webamp — reimplementação do Winamp 2.0
para browser (JS + WebAudio). Referência para o sfrega.

## Funcionalidades do Winamp 2 (extraídas do webamp)

### Transporte / playback
- Play, pause, stop, anterior, próxima
- Seek na barra de posição (clique/arraste)
- Tempo decorrido / total, marquee no título
- Modos: repeat (off/all/one), shuffle, A-B repeat
- Crossfade entre faixas
- Fade in/out

### Áudio
- Volume + mute (slider + scroll do mouse)
- Balance (pan L/R)
- Stereo/mono toggle
- **Equalizador gráfico**: presets de 10, 18, 20, 26 e 31 bandas (EQF)
- Pré-amp / gain
- **Spectrum analyzer**: presets spec1..spec32 (estilos visuais diferentes)
- Osciloscópio (waveform)
-VU meter

### Playlist
- Playlist editor (janela separada): add/remove/rename/reorder
- Salvar/carregar .m3u, .pls, .asx, .ram, .rpm
- Autoscroll, seleção por teclado, play-on-select
- Drag & drop de arquivos

### Biblioteca / media
- Media library: artists, álbuns, gêneros, ano, comentários
- Gerar playlist a partir de filtros da biblioteca
- Scrobble (Last.fm / libre.fm)

### Visual / skins
- Sistema de skins (arquivos .wal em zip)
- Main window, equalizer window, playlist window, region free, about
- Modo compacto / always on top
- Video support (versões posteriores)

### Sistema
- Plugins: input (formatos), dsp (efeitos), output (drivers), visualization
- Configuração persistente (winamp.ini equivalente)
- Minimizar para tray, atalhos globais

## Mapeamento para o sfrega (Go, multi-plataforma)

| Área | Status |
|---|---|
| Decoder multi-formato | ✅ miniaudio (wav/mp3/flac/ogg/m4a/aac...) |
| Saída de áudio | ✅ miniaudio (wasapi/alsa/coreaudio/oboe) |
| Playlist básico | ✅ player.SetTracks |
| Spectrum analyzer | ✅ FFT 2048 → 48 bandas |
| Volume | ✅ device gain |
| Seek | ✅ dec_seek_pcm |
| GUI estilo Winamp | ⬜ próxima etapa (Gio ou webview) |
| EQ / presets | ⬜ DSP no ring buffer |
| Balance/mono | ⬜ simples no callback |
| .m3u/.pls | ⬜ parser |
| Skins | ⬜ |
| Media library | ⬜ |
| iOS | ⚠️ miniaudio suporta; testar com gomobile |

## Arquitetura sfrega

```
arquivo → audio.Decoder (miniaudio) → PCM f32
       → spectrum.Analyzer (FFT) → bandas p/ UI
       → audio.RingBuffer → audio.Device (callback C) → saída
```

- `internal/audio` — wrapper CGo do miniaudio (decoder, ring buffer, device)
- `internal/player` — playlist, transporte, pipeline
- `internal/spectrum` — FFT radix-2 + bandas logarítmicas
- `cmd/sfrega` — CLI (placeholder até a GUI)
