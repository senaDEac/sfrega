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

// Feed alimenta o analisador com PCM mono float32 e retorna Bands magnitudes
// normalizadas (0..1, escala log). O PCM deve ter len > 0.
func (a *Analyzer) Feed(pcm []float32, sampleRate uint32) [Bands]float32 {
	var out [Bands]float32
	// downmix para mono se estéreo (pcm intercalado)
	mono := make([]float32, FFTSize)
	step := len(pcm) / FFTSize
	if step < 1 {
		step = 1
	}
	for i := 0; i < FFTSize; i++ {
		idx := i * step
		if idx >= len(pcm) {
			break
		}
		v := pcm[idx]
		// assume estéreo: pega também o canal seguinte se houver
		if idx+1 < len(pcm) {
			v = (v + pcm[idx+1]) * 0.5
		}
		mono[i] = v * a.window[i]
	}
	for i := 0; i < FFTSize; i++ {
		a.fftBuf[i] = complex(float64(mono[i]), 0)
	}
	fft(a.fftBuf)
	// magnitude -> bandas logarítmicas
	half := FFTSize / 2
	for b := 0; b < Bands; b++ {
		// mapeia banda para faixa de frequência log (20Hz .. Nyquist)
		nyq := float64(sampleRate) / 2
		fLo := 20.0 * math.Pow(nyq/20.0, float64(b)/float64(Bands))
		fHi := 20.0 * math.Pow(nyq/20.0, float64(b+1)/float64(Bands))
		kLo := int(fLo / float64(sampleRate) * float64(FFTSize))
		kHi := int(fHi / float64(sampleRate) * float64(FFTSize))
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
		avg := peak * 2.0 / float64(FFTSize) * 4.0
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
		a.prev[b] = a.prev[b]*a.smooth + v*(1-a.smooth)
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
