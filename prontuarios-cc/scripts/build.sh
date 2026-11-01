#!/usr/bin/env bash
#
# Compila o chaincode e o patient-signer (estáticos, linux/amd64) e monta o
# pacote CCaaS em build/. O pacote contém apenas o endereço do serviço; o
# código é executado fora do peer, como serviço systemd em cada máquina.

set -e

cd "$(dirname "$0")/.."

LABEL=${CC_LABEL:-prontuarios_1.0}
CC_PORT=${CC_PORT:-9999}

mkdir -p build
(
    cd chaincode
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../build/prontuarios-cc .
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../build/patient-signer ./cmd/patient-signer
)

# Pacote no formato do builder ccaas: metadata.json + code.tar.gz
# (connection.json). Cada peer conecta ao serviço local da própria máquina.
TMP=$(mktemp -d)
trap 'rm -rf ${TMP}' EXIT
cat > ${TMP}/connection.json <<JSON
{
  "address": "127.0.0.1:${CC_PORT}",
  "dial_timeout": "10s",
  "tls_required": false
}
JSON
cat > ${TMP}/metadata.json <<JSON
{
  "type": "ccaas",
  "label": "${LABEL}"
}
JSON
# Empacotamento determinístico: o package ID é o SHA-256 do pacote e deve ser
# o mesmo a cada compilação.
reproducible_tar() {
    local out=$1
    shift
    tar -C ${TMP} --sort=name --mtime='2026-01-01 00:00:00Z' --owner=0 --group=0 --numeric-owner \
        -cf - "$@" | gzip -n > ${out}
}
reproducible_tar ${TMP}/code.tar.gz connection.json
touch -d '2026-01-01 00:00:00Z' ${TMP}/code.tar.gz
reproducible_tar build/prontuarios-cc.tar.gz metadata.json code.tar.gz

# Segredo do HMAC do CPF: gerado uma única vez e idêntico em todos os peers.
if [ ! -f build/pepper ]; then
    head -c 32 /dev/urandom | base64 > build/pepper
    chmod 600 build/pepper
fi

echo "${LABEL}:$(sha256sum build/prontuarios-cc.tar.gz | cut -d' ' -f1)" > build/package-id
echo "Pacote: build/prontuarios-cc.tar.gz"
echo "Package ID: $(cat build/package-id)"
