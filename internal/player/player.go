// Package player implementa o núcleo do player: playlist, transporte,
// pipeline de decode → análise de espectro → ring buffer → dispositivo.
package player

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"sfrega/internal/audio"
	"sfrega/internal/spectrum"
)

const (
	decodeChunkFrames = 4096
	ringSeconds       = 2 // ~2s de buffer
)

// Event é emitido para a UI.
type Event struct {
	Kind     string // "track-change", "state-change", "eof", "error", "spectrum"
	Index    int
	Title    string
	Playing  bool
	Volume   float32
	Duration time.Duration
	Bands    [spectrum.Bands]float32
	Err      error
}

// Player é o núcleo do player.
type Player struct {
	mu       sync.Mutex
	tracks   []string
	index    int
	dec      *audio.Decoder
	rb       *audio.RingBuffer
	dev      *audio.Device
	analyzer *spectrum.Analyzer

	playing   atomic.Bool
	volume    atomic.Uint32 // float32 bits
	decoded   atomic.Uint64 // frames decodificados totais da faixa
	done      chan struct{}
	trackDone chan struct{} // cancelado ao trocar de faixa
	events    chan Event
	lastBands atomic.Value // [spectrum.Bands]float32
}

// New cria um Player. rate/channels são do dispositivo de saída.
func New(rate uint32, channels uint32) (*Player, error) {
	capSamples := uint64(rate) * uint64(channels) * ringSeconds
	rb := audio.NewRingBuffer(capSamples)
	if rb == nil {
		return nil, errors.New("player: falha ao criar ring buffer")
	}
	p := &Player{
		rb:       rb,
		done:     make(chan struct{}),
		events:   make(chan Event, 64),
		analyzer: spectrum.New(),
	}
	p.volume.Store(math.Float32bits(1.0))
	return p, nil
}

func (p *Player) Events() <-chan Event { return p.events }

func (p *Player) emit(e Event) {
	select {
	case p.events <- e:
	default:
	}
}

// SetTracks define a playlist.
func (p *Player) SetTracks(tracks []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tracks = append([]string(nil), tracks...)
	p.index = -1
	if len(tracks) > 0 {
		p.index = 0
	}
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

func (p *Player) currentTitle() string {
	if p.index >= 0 && p.index < len(p.tracks) {
		return p.tracks[p.index]
	}
	return ""
}

func (p *Player) Index() int    { return p.index }
func (p *Player) Count() int    { return len(p.tracks) }
func (p *Player) Playing() bool { return p.playing.Load() }

// Volume retorna 0..1.
func (p *Player) Volume() float32 { return math.Float32frombits(p.volume.Load()) }

// SetVolume define volume 0..1.
func (p *Player) SetVolume(v float32) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	p.volume.Store(math.Float32bits(v))
	p.mu.Lock()
	if p.dev != nil {
		p.dev.SetVolume(v)
	}
	p.mu.Unlock()
	p.emit(Event{Kind: "state-change", Volume: v, Playing: p.playing.Load()})
}

// Play inicia a faixa atual.
func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index < 0 || p.index >= len(p.tracks) {
		return errors.New("player: playlist vazia")
	}
	if p.dec == nil {
		if err := p.openCurrentLocked(); err != nil {
			return err
		}
	}
	if !p.playing.Load() {
		p.playing.Store(true)
		p.emit(Event{Kind: "state-change", Playing: true, Volume: p.Volume()})
	}
	return nil
}

func (p *Player) Pause() {
	if p.playing.CompareAndSwap(true, false) {
		p.emit(Event{Kind: "state-change", Playing: false, Volume: p.Volume()})
	}
}

func (p *Player) Toggle() {
	if p.playing.Load() {
		p.Pause()
	} else {
		_ = p.Play()
	}
}

// Next avança; no fim da playlist para.
func (p *Player) Next() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index+1 < len(p.tracks) {
		p.index++
		p.playNextLocked()
	} else {
		p.index = -1
		p.resetLocked()
	}
}

// Prev volta.
func (p *Player) Prev() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index-1 >= 0 {
		p.index--
		p.playNextLocked()
	} else {
		p.resetLocked()
	}
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

// playNextLocked assume lock; troca de faixa mantendo estado de play.
func (p *Player) playNextLocked() {
	wasPlaying := p.playing.Load()
	p.closeTrackLocked()
	p.decoded.Store(0)
	if wasPlaying {
		p.playing.Store(false)
		_ = p.openCurrentLocked()
		p.playing.Store(true)
	} else {
		p.playing.Store(false)
	}
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

func (p *Player) resetLocked() {
	p.closeTrackLocked()
	p.playing.Store(false)
	p.decoded.Store(0)
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

func (p *Player) closeTrackLocked() {
	if p.trackDone != nil {
		close(p.trackDone)
		p.trackDone = nil
	}
	if p.dec != nil {
		p.dec.Close()
		p.dec = nil
	}
	if p.dev != nil {
		p.dev.Close()
		p.dev = nil
	}
}

func (p *Player) openCurrentLocked() error {
	path := p.tracks[p.index]
	dec, err := audio.OpenDecoder(path)
	if err != nil {
		p.emit(Event{Kind: "error", Err: err, Index: p.index})
		return err
	}
	rate := dec.SampleRate()
	ch := dec.Channels()
	dev, err := audio.OpenDevice(rate, ch, p.rb)
	if err != nil {
		dec.Close()
		return err
	}
	dev.SetVolume(p.Volume())
	p.dec = dec
	p.dev = dev
	if !dev.Start() {
		dec.Close()
		dev.Close()
		p.dec, p.dev = nil, nil
		return errors.New("player: falha ao iniciar dispositivo")
	}
	p.decoded.Store(0)
	p.decoded.Store(0)
	p.trackDone = make(chan struct{})
	p.emit(Event{Kind: "track-change", Index: p.index, Title: path,
		Duration: time.Duration(dec.LengthFrames()) * time.Second / time.Duration(rate)})
	go p.decodeLoop(dec, rate, ch, p.trackDone)
	return nil
}

// decodeLoop decodifica a faixa, alimenta analisador e ring buffer.
func (p *Player) decodeLoop(dec *audio.Decoder, rate, channels uint32, stop <-chan struct{}) {
	chBuf := make([]float32, decodeChunkFrames*channels)
	for {
		select {
		case <-p.done:
			return
		case <-stop:
			return
		default:
		}
		if !p.playing.Load() {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		frames := dec.ReadPCM(chBuf, decodeChunkFrames)
		if frames == 0 {
			p.emit(Event{Kind: "eof", Index: p.index})
			return
		}
		samples := int(frames) * int(channels)
		chunk := chBuf[:samples]
		bands := p.analyzer.Feed(chunk, rate)
		p.lastBands.Store(bands)
		// escreve no ring buffer (parcial + espera)
		written := 0
		for written < samples {
			n := p.rb.Write(chunk[written:])
			if n == 0 {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			written += n
		}
		p.decoded.Add(uint64(frames))
	}
}

// playedFrames estima frames já tocados = decodificados - buffer restante.
func (p *Player) playedFrames(rate, channels uint32) uint64 {
	dec := p.decoded.Load()
	avail := p.rb.Available()
	ch := uint64(channels)
	if ch == 0 {
		ch = 2
	}
	buffered := avail / ch
	if dec > buffered {
		return dec - buffered
	}
	return 0
}

// PositionFrames retorna posição aproximada em frames.
func (p *Player) PositionFrames() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return p.playedFrames(p.dec.SampleRate(), p.dec.Channels())
}

// SeekFrame posiciona em frame de PCM.
func (p *Player) SeekFrame(frame int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return errors.New("player: nada tocando")
	}
	if !p.dec.SeekFrame(frame) {
		return errors.New("player: seek falhou")
	}
	p.decoded.Store(uint64(frame))
	return nil
}

// Spectrum retorna o último espectro calculado.
func (p *Player) Spectrum() [spectrum.Bands]float32 {
	if v, ok := p.lastBands.Load().([spectrum.Bands]float32); ok {
		return v
	}
	return [spectrum.Bands]float32{}
}

// Duration retorna duração da faixa atual.
func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return time.Duration(p.dec.LengthFrames()) * time.Second / time.Duration(p.dec.SampleRate())
}

// Position retorna posição atual (aprox., baseada em frames escritos no buffer
// menos o buffer restante — estimativa simples por pos de decode).
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return time.Duration(p.playedFrames(p.dec.SampleRate(), p.dec.Channels())) * time.Second / time.Duration(p.dec.SampleRate())
}

// Stats retorna (frames decodificados, frames no buffer, frames tocados).
func (p *Player) Stats() (decoded, buffered, played uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0, 0, 0
	}
	ch := p.dec.Channels()
	if ch == 0 {
		ch = 2
	}
	return p.decoded.Load(), p.rb.Available() / uint64(ch), p.playedFrames(p.dec.SampleRate(), ch)
}

// Close libera recursos.
func (p *Player) Close() {
	close(p.done)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeTrackLocked()
	p.rb.Destroy()
}
