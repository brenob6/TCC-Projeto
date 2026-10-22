# !/usr/bin/sh

set -e

IPFS_VERSION="0.42.0"

mkdir -p downloads

wget https://dist.ipfs.tech/kubo/v${IPFS_VERSION}/kubo_v${IPFS_VERSION}_linux-amd64.tar.gz -O downloads/kubo.tar.gz
