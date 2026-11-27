package audio

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"
	"unsafe"
)

// StreamDecoder decodifica um stream HTTP ao vivo (rádio) através do
// ffmpeg do sistema: a saída é PCM float32 little-endian estereofônico a
// 44100 Hz, no mesmo formato que o Decoder entrega para o ring buffer.
//
// Requer ffmpeg no PATH. Streams HLS (m3u8), shoutcast, icecast etc. são
// tratados pelo ffmpeg (inclui reconexão automática).
type StreamDecoder struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	r        *bufio.Reader
	stderr   *bytes.Buffer
	path     string
	icyTitle string
	icyDone  chan struct{}
	icyOnce  sync.Once
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
	s := &StreamDecoder{
		cmd:     cmd,
		r:       bufio.NewReaderSize(stdout, 1<<16),
		stderr:  stderr,
		path:    url,
		icyDone: make(chan struct{}),
	}
	// segunda conexão só para ler o StreamTitle ICY (o ffmpeg consome os
	// metadados sem expô-los); custo ~1 stream a mais de largura de banda
	go s.sniffICY()
	return s, nil
}

// ICYTitle devolve o último StreamTitle ('música corrente') lido do
// stream, ou "" se a estação não envia metadados.
func (s *StreamDecoder) ICYTitle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.icyTitle
}

// setICYTitle guarda o título mais recente.
func (s *StreamDecoder) setICYTitle(t string) {
	s.mu.Lock()
	s.icyTitle = t
	s.mu.Unlock()
}

// sniffICY mantém uma conexão HTTP paralela varrendo blocos
// `StreamTitle='...';` embutidos no stream (protocolo ICY/Shoutcast).
func (s *StreamDecoder) sniffICY() {
	for {
		select {
		case <-s.icyDone:
			return
		default:
		}
		if !s.sniffICYOnce() {
			time.Sleep(3 * time.Second)
		}
	}
}

// sniffICYOnce lê o stream até o servidor fechar; retorna false em erro
// de conexão (o loop externo tenta de novo).
func (s *StreamDecoder) sniffICYOnce() bool {
	req, err := http.NewRequest("GET", s.path, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Icy-MetaData", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	const mark = "StreamTitle='"
	buf := make([]byte, 32*1024)
	var carry []byte
	for {
		select {
		case <-s.icyDone:
			return true
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			carry = append(carry, buf[:n]...)
			// varre todos os títulos no bloco recebido
			for {
				i := bytes.Index(carry, []byte(mark))
				if i < 0 {
					break
				}
				rest := carry[i+len(mark):]
				j := bytes.Index(rest, []byte("';"))
				if j < 0 {
					break // título dividido entre leituras: aguarda mais dados
				}
				s.setICYTitle(string(rest[:j]))
				carry = rest[j+2:]
			}
			// mantém só o rabo para não crescer sem limite
			if len(carry) > 4096 {
				carry = append([]byte(nil), carry[len(carry)-256:]...)
			}
		}
		if err != nil {
			return false
		}
	}
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

// Close encerra o processo ffmpeg e o sniffer ICY.
func (s *StreamDecoder) Close() {
	s.icyOnce.Do(func() { close(s.icyDone) })
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
