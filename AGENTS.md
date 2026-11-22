# AGENTS.md — sfrega

## Projeto
Aplicação em **Go** multi-plataforma: Windows, Linux, macOS, Android, iOS.

## Regras
- Escreva código Go idiomático; verifique com `go build ./...` e `go vet ./...`.
- Cross-compile por GOOS/GOARCH (ex.: `GOOS=windows GOARCH=amd64`).
- Android: `GOOS=android`; iOS: `GOOS=ios` + gomobile/cgomobile quando aplicável.
- Não commit secrets, binários ou artefatos de build.
- Detalhes de ambiente local em `00-install/notas.md`.
