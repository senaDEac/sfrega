// Package audio é o backend de áudio do sfrega, baseado em miniaudio.
// Suporta Windows, Linux, macOS, Android e iOS.
package audio

/*
#cgo CFLAGS: -I.
#cgo LDFLAGS: -lm
#cgo linux LDFLAGS: -ldl
#include "audio.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Decoder decodifica um arquivo de áudio para PCM float intercalado.
// miniaudio não é thread-safe: todos os acessos passam por mu.
type Decoder struct {
	mu   sync.Mutex
	c    *C.ma_dec_t
	path string
}

// OpenDecoder abre um arquivo de áudio (wav, mp3, flac, ogg, etc.).
func OpenDecoder(path string) (*Decoder, error) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	c := C.dec_open(cp)
	if c == nil {
		return nil, errors.New("audio: falha ao abrir " + path)
	}
	return &Decoder{c: c, path: path}, nil
}

func (d *Decoder) LengthFrames() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return 0
	}
	return uint64(C.dec_get_length_pcm(d.c))
}
func (d *Decoder) Channels() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return 0
	}
	return uint32(C.dec_get_channels(d.c))
}
func (d *Decoder) SampleRate() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return 0
	}
	return uint32(C.dec_get_sample_rate(d.c))
}
func (d *Decoder) Path() string { return d.path }

// ReadPCM lê até frames frames para buf (float32 intercalado).
// Retorna frames efetivamente lidos (0 = fim).
func (d *Decoder) ReadPCM(buf []float32, frames uint64) uint64 {
	if len(buf) == 0 || frames == 0 {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return 0
	}
	return uint64(C.dec_read_f32(d.c, (*C.float)(unsafe.Pointer(&buf[0])), C.uint64_t(frames)))
}

func (d *Decoder) SeekFrame(frame int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c == nil {
		return false
	}
	return bool(C.dec_seek_pcm(d.c, C.int64_t(frame)))
}

func (d *Decoder) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.c != nil {
		C.dec_close(d.c)
		d.c = nil
	}
}

// RingBuffer é um buffer circular de amostras float intercaladas.
type RingBuffer struct {
	c *C.ma_rb_t
}

func NewRingBuffer(capacitySamples uint64) *RingBuffer {
	c := C.rb_create(C.uint64_t(capacitySamples))
	if c == nil {
		return nil
	}
	return &RingBuffer{c: c}
}

// Write escreve samples; retorna quantos foram escritos (pode ser parcial se cheio).
func (rb *RingBuffer) Write(data []float32) int {
	n := uint64(len(data))
	if n == 0 {
		return 0
	}
	return int(C.rb_write(rb.c, (*C.float)(unsafe.Pointer(&data[0])), C.uint64_t(n)))
}

func (rb *RingBuffer) Read(data []float32) int {
	n := uint64(len(data))
	if n == 0 {
		return 0
	}
	return int(C.rb_read(rb.c, (*C.float)(unsafe.Pointer(&data[0])), C.uint64_t(n)))
}

func (rb *RingBuffer) Available() uint64 {
	return uint64(C.rb_available(rb.c))
}

// Clear esvazia o buffer imediatamente.
func (rb *RingBuffer) Clear() {
	C.rb_clear(rb.c)
}

func (rb *RingBuffer) Destroy() {
	if rb.c != nil {
		C.rb_destroy(rb.c)
		rb.c = nil
	}
}

// Device é o dispositivo de saída de áudio.
type Device struct {
	c *C.ma_dev_t
}

func OpenDevice(sampleRate, channels uint32, rb *RingBuffer) (*Device, error) {
	c := C.dev_open(C.uint32_t(sampleRate), C.uint32_t(channels), rb.c)
	if c == nil {
		return nil, errors.New("audio: falha ao abrir dispositivo de saída")
	}
	return &Device{c: c}, nil
}

func (d *Device) Start() bool { return bool(C.dev_start(d.c)) }
func (d *Device) Stop()       { C.dev_stop(d.c) }

func (d *Device) SetVolume(v float32) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	C.dev_set_volume(d.c, C.float(v))
}

func (d *Device) Volume() float32 { return float32(C.dev_get_volume(d.c)) }

// SetPan define o balanço -1 (esquerda) .. +1 (direita), aplicado na saída.
func (d *Device) SetPan(p float32) {
	if p < -1 {
		p = -1
	}
	if p > 1 {
		p = 1
	}
	C.dev_set_pan(d.c, C.float(p))
}

func (d *Device) Close() {
	if d.c != nil {
		C.dev_stop(d.c)
		C.dev_close(d.c)
		d.c = nil
	}
}
