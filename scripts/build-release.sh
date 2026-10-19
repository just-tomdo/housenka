#!/bin/sh
# Cross-compiles the release binaries into dist/ together with checksums.txt.
# Asset names carry no version, so install.sh can always fetch the latest one.
#
# Usage: scripts/build-release.sh v1.2.3
set -eu

version=${1:?"použití: scripts/build-release.sh VERZE (např. v1.2.3)"}

cd "$(dirname "$0")/.."
rm -rf dist
mkdir dist

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os=${target%/*}
	arch=${target#*/}
	out=dist/housenka-$os-$arch
	[ "$os" = windows ] && out=$out.exe
	echo "sestavuji $out"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
		go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out" .
done

(cd dist && sha256sum housenka-* >checksums.txt)
