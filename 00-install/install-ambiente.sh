#!/usr/bin/env bash
# Configura ambiente local: GOPATH local, PATH e diretórios do projeto.
# Uso: ./install-ambiente.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export GOPATH="$ROOT/.gopath"
mkdir -p "$GOPATH" "$ROOT/bin"

# Sugestões para ~/.bashrc (idempotente)
BLOCK='
# sfrega
export GOPATH="'"$GOPATH"'"
export PATH="$GOPATH/bin:$PATH"
export GOFLAGS="-mod=mod"
'

if ! grep -q "# sfrega" ~/.bashrc 2>/dev/null; then
  printf '%s\n' "$BLOCK" >> ~/.bashrc
  echo "Adicionado bloco ao ~/.bashrc — recarregue com: source ~/.bashrc"
else
  echo "Bloco já presente no ~/.bashrc"
fi

echo "Ambiente OK. GOPATH=$GOPATH"
