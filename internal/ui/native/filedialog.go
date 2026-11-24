package native

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// pickAudioFiles abre o diálogo nativo de arquivos do sistema e retorna
// os caminhos selecionados filtrados por extensão de áudio.
// Retorna nil se o usuário cancelar ou a plataforma não suportar.
func pickAudioFiles() []string {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "linux":
		out, err = pickLinux()
	case "windows":
		out, err = pickWindows()
	case "darwin":
		out, err = pickDarwin()
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	var res []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if audioExts[strings.ToLower(filepath.Ext(line))] {
			res = append(res, line)
		}
	}
	return res
}

var audioExts = map[string]bool{
	".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".m4a": true,
	".aac": true, ".wma": true, ".opus": true, ".aiff": true, ".ape": true,
}

// pickLinux tenta zenity e depois kdialog.
func pickLinux() ([]byte, error) {
	filter := "Audio|*.mp3 *.wav *.flac *.ogg *.m4a *.aac *.wma *.opus *.aiff *.ape"
	if _, err := exec.LookPath("zenity"); err == nil {
		return exec.Command("zenity", "--file-selection", "--multiple",
			"--separator=\n", "--file-filter", filter).Output()
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		return exec.Command("kdialog", "--getopenfilename", ".",
			filter, "--multiple", "--separate-output").Output()
	}
	return nil, exec.ErrNotFound
}

// pickWindows usa OpenFileDialog via PowerShell.
func pickWindows() ([]byte, error) {
	ps := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$d=New-Object System.Windows.Forms.OpenFileDialog;` +
		`$d.Multiselect=$true;` +
		`$d.Filter='Audio|*.mp3;*.wav;*.flac;*.ogg;*.m4a;*.aac;*.wma;*.opus;*.aiff;*.ape';` +
		`if($d.ShowDialog() -eq 'OK'){$d.FileNames -join [string][char]10}`
	return exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
}

// pickDarwin usa o comando choose file do AppleScript.
func pickDarwin() ([]byte, error) {
	return exec.Command("osascript", "-e",
		`set fs to choose file with prompt "sfrega" with multiple selections allowed`,
		"-e", `set out to ""`,
		"-e", `repeat with f in fs`,
		"-e", `set out to out & POSIX path of f & linefeed`,
		"-e", `end repeat`,
		"-e", `return out`).Output()
}
