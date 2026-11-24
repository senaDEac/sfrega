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

// eqBandFreqs são as frequências centrais do equalizador de 10 bandas (ISO).
var eqBandFreqs = [10]float64{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// biquad é um filtro biquad (RBJ peaking EQ).
type biquad struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             float64
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}

func (b *biquad) reset() { b.z1, b.z2 = 0, 0 }

// eqState contém os filtros EQ por banda e canal.
type eqState struct {
	filters [10][2]biquad
}

// eqFromGains calcula coeficientes para ganhos em dB (len deve ser 10).
func eqFromGains(gainsDb [10]float64, sampleRate uint32) *eqState {
	e := &eqState{}
	for b := 0; b < 10; b++ {
		e.filters[b][0] = biquad{b0: 1} // identity (bypass)
		e.filters[b][1] = biquad{b0: 1}
	}
	Q := 1.4
	for band := 0; band < 10; band++ {
		db := gainsDb[band]
		if math.Abs(db) < 0.01 {
			continue // bypass
		}
		A := math.Pow(10, db/40)
		w0 := 2 * math.Pi * eqBandFreqs[band] / float64(sampleRate)
		cosw := math.Cos(w0)
		sinw := math.Sin(w0)
		alpha := sinw / (2 * Q)
		b0 := 1 + alpha*A
		b1 := -2 * cosw
		b2 := 1 - alpha*A
		a0 := 1 + alpha/A
		a1 := -2 * cosw
		a2 := 1 - alpha/A
		for ch := 0; ch < 2; ch++ {
			e.filters[band][ch] = biquad{
				b0: b0 / a0, b1: b1 / a0, b2: b2 / a0,
				a1: a1 / a0, a2: a2 / a0,
			}
		}
	}
	return e
}

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
	repeat    atomic.Bool
	volume    atomic.Uint32 // float32 bits
	eq        atomic.Value  // *eqState
	preamp    atomic.Uint32 // float32 bits (ganho pré-EQ em dB, clamp ±12)
	pan       atomic.Uint32 // float32 bits (-1 esquerda .. +1 direita)
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
	p.preamp.Store(math.Float32bits(1))
	p.pan.Store(math.Float32bits(0))
	p.eq.Store(eqFromGains([10]float64{}, 44100))
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

// AddTracks anexa faixas ao fim da playlist sem interromper a reprodução.
func (p *Player) AddTracks(tracks []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(tracks) == 0 {
		return
	}
	p.tracks = append(p.tracks, tracks...)
	if p.index < 0 {
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

// TrackAt retorna o caminho da faixa i (ou "").
func (p *Player) TrackAt(i int) string {
	if i >= 0 && i < len(p.tracks) {
		return p.tracks[i]
	}
	return ""
}

// PlayIndex toca a faixa i da playlist.
func (p *Player) PlayIndex(i int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.tracks) {
		return errors.New("player: índice inválido")
	}
	p.index = i
	if p.dec != nil {
		p.closeTrackLocked()
		p.decoded.Store(0)
	}
	if err := p.openCurrentLocked(); err != nil {
		return err
	}
	p.playing.Store(true)
	return nil
}

func (p *Player) Index() int    { return p.index }
func (p *Player) Count() int    { return len(p.tracks) }
func (p *Player) Playing() bool { return p.playing.Load() }

// SetRepeat liga/desliga o repeat da playlist.
func (p *Player) SetRepeat(on bool) { p.repeat.Store(on) }

// Repeat retorna o estado do repeat.
func (p *Player) Repeat() bool { return p.repeat.Load() }

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

// SetEQ define os ganhos do equalizador em dB (10 bandas ISO).
// Requer sampleRate para calcular coeficientes; use EQSampleRate ou 0 para
// usar a taxa da faixa atual quando souber.
func (p *Player) SetEQ(gainsDb [10]float64, sampleRate uint32) {
	if sampleRate == 0 {
		p.mu.Lock()
		if p.dec != nil {
			sampleRate = p.dec.SampleRate()
		}
		p.mu.Unlock()
		if sampleRate == 0 {
			sampleRate = 44100
		}
	}
	p.eq.Store(eqFromGains(gainsDb, sampleRate))
}

// EQGains retorna ganhos atuais (se trackeados externamente).
// SetPreamp define ganho pré-EQ em dB (±12).
func (p *Player) SetPreamp(db float64) {
	if db > 12 {
		db = 12
	}
	if db < -12 {
		db = -12
	}
	// converte dB para multiplicador e guarda em volume-like? guardamos o
	// multiplicador linear em preamp (float32 bits)
	p.preamp.Store(math.Float32bits(float32(math.Pow(10, db/20))))
}

func (p *Player) Preamp() float32 { return math.Float32frombits(p.preamp.Load()) }

// SetBalance define o balanço -1 (esquerda) .. +1 (direita).
func (p *Player) SetBalance(b float32) {
	if b < -1 {
		b = -1
	}
	if b > 1 {
		b = 1
	}
	p.pan.Store(math.Float32bits(b))
}

func (p *Player) Balance() float32 { return math.Float32frombits(p.pan.Load()) }

// Channels retorna o número de canais da faixa atual (0 se nenhuma).
func (p *Player) Channels() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return p.dec.Channels()
}

// SeekFraction posiciona em fração 0..1 da duração.
func (p *Player) SeekFraction(f float64) error {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return errors.New("player: nada tocando")
	}
	total := p.dec.LengthFrames()
	if !p.dec.SeekFrame(int64(float64(total) * f)) {
		return errors.New("player: seek falhou")
	}
	p.decoded.Store(uint64(int64(float64(total) * f)))
	return nil
}

// Tracks retorna uma cópia da playlist.
func (p *Player) Tracks() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.tracks...)
}

// RemoveAt remove a faixa i da playlist.
func (p *Player) RemoveAt(i int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.tracks) {
		return
	}
	p.tracks = append(p.tracks[:i], p.tracks[i+1:]...)
	if len(p.tracks) == 0 {
		p.index = -1
		p.resetLocked()
		return
	}
	if i < p.index {
		p.index--
	} else if i == p.index {
		p.index = -1
		p.index = 0
		if p.dec != nil {
			p.closeTrackLocked()
			p.decoded.Store(0)
		}
	}
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

// Move move a faixa de from para to.
func (p *Player) Move(from, to int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if from < 0 || from >= len(p.tracks) || to < 0 || to >= len(p.tracks) || from == to {
		return
	}
	t := p.tracks[from]
	p.tracks = append(p.tracks[:from], p.tracks[from+1:]...)
	p.tracks = append(p.tracks[:to], append([]string{t}, p.tracks[to:]...)...)
	if p.index == from {
		p.index = to
	} else if from < p.index && to >= p.index {
		p.index--
	} else if from > p.index && to <= p.index {
		p.index++
	}
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
		p.rb.Clear()
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

// decodeLoop decodifica a faixa, aplica EQ/preamp, alimenta analisador e ring buffer.
func (p *Player) decodeLoop(dec *audio.Decoder, rate, channels uint32, stop <-chan struct{}) {
	chBuf := make([]float32, decodeChunkFrames*channels)
	eq := p.eq.Load().(*eqState)
	preamp := p.Preamp()
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
			p.autoAdvance()
			return
		}
		samples := int(frames) * int(channels)
		chunk := chBuf[:samples]
		// EQ + preamp (recarrega estado a cada chunk p/ sliders ao vivo)
		if eq2, ok := p.eq.Load().(*eqState); ok && eq2 != eq {
			eq = eq2
		}
		preamp = p.Preamp()
		pan := p.Balance()
		if channels == 2 {
			gl, gr := 1.0, 1.0
			if pan < 0 {
				gr = float64(1 + pan)
			} else {
				gl = float64(1 - pan)
			}
			for i := 0; i < samples; i += 2 {
				l := float64(chunk[i])
				r := float64(chunk[i+1])
				for b := 0; b < 10; b++ {
					l = eq.filters[b][0].process(l)
					r = eq.filters[b][1].process(r)
				}
				chunk[i] = float32(l * float64(preamp) * gl)
				chunk[i+1] = float32(r * float64(preamp) * gr)
			}
		} else {
			for i := 0; i < samples; i++ {
				l := float64(chunk[i])
				for b := 0; b < 10; b++ {
					l = eq.filters[b][0].process(l)
				}
				chunk[i] = float32(l * float64(preamp))
			}
		}
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

// autoAdvance decide o que fazer ao fim da faixa: próxima faixa, wrap
// com repeat, ou volta ao início pausado.
func (p *Player) autoAdvance() {
	p.mu.Lock()
	n := len(p.tracks)
	hasNext := p.index+1 < n
	rep := p.repeat.Load()
	p.mu.Unlock()
	switch {
	case hasNext:
		p.Next()
	case rep && n > 0:
		_ = p.PlayIndex(0)
	default:
		_ = p.SeekFrame(0)
		p.Pause()
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
	p.rb.Clear()
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
