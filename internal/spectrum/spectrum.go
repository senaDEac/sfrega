// Package spectrum implementa análise de espectro (FFT) para o visualizador.
package spectrum

import "math"

const (
	// FFTSize é o tamanho da janela de análise (deve ser potência de 2).
	FFTSize = 2048
	// Bands é o número de barras do espectro.
	Bands = 48
)

// Analyzer mantém estado do espectro (janela + suavização).
type Analyzer struct {
	window [FFTSize]float32
	fftBuf []complex128
	prev   [Bands]float32
	smooth float32
}

// New cria um Analyzer com janela de Hann.
func New() *Analyzer {
	a := &Analyzer{
		fftBuf: make([]complex128, FFTSize),
		smooth: 0.55,
	}
	for i := 0; i < FFTSize; i++ {
		a.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(FFTSize-1)))
	}
	return a
}

// Feed alimenta o analisador com PCM intercalado (channels por frame) e
// retorna Bands magnitudes normalizadas (0..1, escala log). Janela de
// FFTSize frames; se o bloco for menor, o restante fica zerado (padding).
func (a *Analyzer) Feed(pcm []float32, channels int, sampleRate uint32) [Bands]float32 {
	var out [Bands]float32
	if channels <= 0 {
		channels = 1
	}
	frames := len(pcm) / channels
	if frames <= 0 {
		return out
	}
	// decima no domínio de frames (nunca entrelaça canais)
	step := frames / FFTSize
	if step < 1 {
		step = 1
	}
	filled := 0
	for i := 0; i < FFTSize; i++ {
		f := i * step
		if f >= frames {
			break
		}
		var v float32
		if channels == 2 {
			v = (pcm[2*f] + pcm[2*f+1]) * 0.5
		} else {
			v = pcm[f*channels]
		}
		a.fftBuf[i] = complex(float64(v*a.window[i]), 0)
		filled = i + 1
	}
	for i := filled; i < FFTSize; i++ {
		a.fftBuf[i] = 0
	}
	fft(a.fftBuf)
	// taxa efetiva após decimação (define o Nyquist do eixo de bandas)
	effRate := float64(sampleRate) / float64(step)
	// magnitude -> bandas logarítmicas
	half := FFTSize / 2
	for b := 0; b < Bands; b++ {
		// mapeia banda para faixa de frequência log (20Hz .. Nyquist)
		nyq := effRate / 2
		fLo := 20.0 * math.Pow(nyq/20.0, float64(b)/float64(Bands))
		fHi := 20.0 * math.Pow(nyq/20.0, float64(b+1)/float64(Bands))
		kLo := int(fLo / effRate * float64(FFTSize))
		kHi := int(fHi / effRate * float64(FFTSize))
		if kLo < 1 {
			kLo = 1
		}
		if kHi <= kLo {
			kHi = kLo + 1
		}
		if kHi > half {
			kHi = half
		}
		if kHi <= kLo {
			a.prev[b] *= a.smooth
			out[b] = a.prev[b]
			continue
		}
		var peak float64
		for k := kLo; k < kHi && k < half; k++ {
			m := cmplxAbs(a.fftBuf[k])
			if m > peak {
				peak = m
			}
		}
		// normaliza pelo nº de amostras preenchidas (janela pode estar
		// meio vazia quando o bloco < FFTSize — zero-padding não muda o peak)
		avg := peak * 2.0 / float64(max(filled, 1)) * 4.0
		// compressão perceptual + suavização temporal
		v := float32(math.Pow(avg, 0.6))
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		if v < 0 {
			v = 0
		}
		// atenua os graves: peso sobe de 0.55 (banda 0) a 1.0 (banda 12+);
		// bandas 12-23 (região das barras 3-4 do VU-leds) com desconto leve
		if b < 12 {
			v *= float32(0.55 + 0.45*float64(b)/12)
		} else if b < 24 {
			v *= 0.9
		}
		// ataque rápido, decaimento mais lento (visual tipo webamp)
		sm := float32(0.4)
		if v < a.prev[b] {
			sm = 0.69 // queda ~20% mais rápida que 0.74
		}
		a.prev[b] = a.prev[b]*sm + v*(1-sm)
		out[b] = a.prev[b]
	}
	return out
}

func cmplxAbs(c complex128) float64 {
	return math.Hypot(real(c), imag(c))
}

// fft: radix-2 Cooley-Tukey in-place.
func fft(a []complex128) {
	n := len(a)
	// bit reversal
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wlen := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += length {
			w := complex(1.0, 0)
			for j := i; j < i+length/2; j++ {
				u := a[j]
				v := a[j+length/2] * w
				a[j] = u + v
				a[j+length/2] = u - v
				w *= wlen
			}
		}
	}
}
