# notas.md — ambiente local

## Sistema
CachyOS (Arch-based), Linux. Go instalado: 1.27.x.

## Ferramentas instaladas por `install-ferramentas.sh`
- Go toolchain, git, make
- Cross-compilação: GOOS/GOARCH nativo do Go (sem toolchain extra)
- Android/iOS exigem NDK/Xcode respectivamente (ver `install-ferramentas.sh --full`)

## Scripts
- `install-ferramentas.sh` — instala toolchain e utilitários
- `install-ambiente.sh` — configura variáveis de ambiente e dirs do projeto
- `00-scripts/` — scripts de ajuste local (executáveis)
