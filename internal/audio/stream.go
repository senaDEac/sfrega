package audio

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"sync"
	"unsafe"
)

// StreamDecoder decodifica um stream HTTP ao vivo (rádio) através do
// ffmpeg do sistema: a saída é PCM float32 little-endian estereofônico a
// 44100 Hz, no mesmo formato que o Decoder entrega para o ring buffer.
//
// Requer ffmpeg no PATH. Streams HLS (m3u8), shoutcast, icecast etc. são
// tratados pelo ffmpeg (inclui reconexão automática).
type StreamDecoder struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	r      *bufio.Reader
	stderr *bytes.Buffer
	path   string
}

// OpenStream inicia o ffmpeg para url e devolve o decoder. O processo só
// falha de verdade se o ffmpeg não existir ou a URL for inválida — a
// conexão em si pode demorar (read bloqueia até chegar áudio).
func OpenStream(url string) (*StreamDecoder, error) {
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5",
		"-i", url,
		"-f", "f32le", "-acodec", "pcm_f32le",
		"-ar", "44100", "-ac", "2",
		"pipe:1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &StreamDecoder{
		cmd:    cmd,
		r:      bufio.NewReaderSize(stdout, 1<<16),
		stderr: stderr,
		path:   url,
	}, nil
}

// ReadPCM lê até frames frames estéreo para buf. Retorna os frames
// efetivamente lidos (0 = stream encerrado/falha).
func (s *StreamDecoder) ReadPCM(buf []float32, frames uint64) uint64 {
	if len(buf) == 0 || frames == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.r == nil {
		return 0
	}
	need := int(frames) * 2 * 4 // frames * canais * bytes/f32
	if need > len(buf)*4 {
		need = len(buf) * 4
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), need)
	n, err := io.ReadFull(s.r, raw)
	if n == 0 {
		return 0
	}
	got := uint64(n/4) / 2
	if err != nil && got == 0 {
		return 0
	}
	return got
}

func (s *StreamDecoder) SampleRate() uint32 { return 44100 }
func (s *StreamDecoder) Channels() uint32   { return 2 }
func (s *StreamDecoder) LengthFrames() uint64 {
	return 0 // stream ao vivo: duração desconhecida
}
func (s *StreamDecoder) SeekFrame(frame int64) bool { return false }
func (s *StreamDecoder) Path() string               { return s.path }

// Stderr devolve as últimas mensagens do ffmpeg (diagnóstico).
func (s *StreamDecoder) Stderr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stderr == nil {
		return ""
	}
	return s.stderr.String()
}

// Close encerra o processo ffmpeg.
func (s *StreamDecoder) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.r == nil {
		return
	}
	s.r = nil
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
		s.cmd = nil
	}
}
