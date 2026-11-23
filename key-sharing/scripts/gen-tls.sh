#!/usr/bin/env bash
#
# Gera, no host, os certificados do TLS mútuo entre o coordenador do KMS e os
# custodiantes:
#
#   build/tls/ca.crt                 CA que emitiu todos os certificados
#   build/tls/custodian-N.{crt,key}  certificado de servidor de cada custodiante
#   build/tls/coordinator.{crt,key}  certificado de cliente do coordenador do KMS
#
# A chave privada da CA é descartada após a emissão, de modo que nenhum outro
# certificado pode ser emitido por ela. Se o material já estiver completo, nada
# é alterado; para reemitir, remova build/tls/ e reprovisione os custodiantes.

set -e

cd "$(dirname "$0")/.."

OUT=build/tls
CUSTODIANS=(custodian-1:192.168.56.21 custodian-2:192.168.56.22 custodian-3:192.168.56.23)
DAYS=3650

complete() {
    local entry
    for f in ca.crt coordinator.crt coordinator.key; do
        [ -f ${OUT}/${f} ] || return 1
    done
    for entry in "${CUSTODIANS[@]}"; do
        [ -f ${OUT}/${entry%%:*}.crt ] && [ -f ${OUT}/${entry%%:*}.key ] || return 1
    done
}

if complete; then
    echo "build/tls/ já contém os certificados; mantendo o material atual."
    exit 0
fi

command -v openssl >/dev/null 2>&1 || { echo "ERRO: openssl não encontrado." >&2; exit 1; }

rm -rf ${OUT}
mkdir -p ${OUT}
chmod 700 ${OUT}
WORK=$(mktemp -d)
trap 'rm -rf ${WORK}' EXIT

echo "Gerando a CA do TLS mútuo..."
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
    -keyout ${WORK}/ca.key -out ${OUT}/ca.crt -days ${DAYS} \
    -subj "/O=Prontuarios PoC/CN=CA do KMS e dos custodiantes" \
    -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,keyCertSign,cRLSign" 2>/dev/null

# issue <nome> <extensões>
issue() {
    openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
        -keyout ${OUT}/$1.key -out ${WORK}/$1.csr \
        -subj "/O=Prontuarios PoC/CN=$1" 2>/dev/null
    printf "%b" "$2" > ${WORK}/$1.ext
    openssl x509 -req -in ${WORK}/$1.csr -CA ${OUT}/ca.crt -CAkey ${WORK}/ca.key \
        -CAcreateserial -CAserial ${WORK}/ca.srl -out ${OUT}/$1.crt -days ${DAYS} \
        -extfile ${WORK}/$1.ext 2>/dev/null
    chmod 600 ${OUT}/$1.key
    echo "  $1"
}

echo "Emitindo os certificados:"
for entry in "${CUSTODIANS[@]}"; do
    name=${entry%%:*}
    ip=${entry##*:}
    issue ${name} "basicConstraints=CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:${ip},DNS:${name}\n"
done
issue coordinator "basicConstraints=CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\n"

echo "Certificados em ${OUT}/ (a chave privada da CA foi descartada)."
