#!/usr/bin/env bash
#
# Compila os binários para as máquinas Vagrant (Linux amd64).
# Execute no host antes de `vagrant up`.

set -e

cd "$(dirname "$0")/.."

mkdir -p build

echo "Compilando custodian e kms para linux/amd64..."

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/custodian ./cmd/custodian
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/kms ./cmd/kms

echo ""
echo "Binários gerados em build/:"
ls -lh build/custodian build/kms
