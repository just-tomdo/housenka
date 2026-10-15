#!/bin/sh
# Installs (or updates) housenka from the latest GitHub release:
#
#   curl -fsSL https://raw.githubusercontent.com/just-tomdo/housenka/main/install.sh | sh
#
# HOUSENKA_INSTALL_DIR overrides the target directory (default ~/.local/bin).
# HOUSENKA_RELEASE_URL overrides where the binaries come from (for testing).
set -eu

repo=just-tomdo/housenka
dir=${HOUSENKA_INSTALL_DIR:-$HOME/.local/bin}
base=${HOUSENKA_RELEASE_URL:-https://github.com/$repo/releases/latest/download}

fail() {
	echo "housenka: $*" >&2
	exit 1
}

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "nepodporovaný systém $(uname -s); na Windows stáhni housenka-windows-amd64.exe z https://github.com/$repo/releases" ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "nepodporovaná architektura $(uname -m)" ;;
esac
asset=housenka-$os-$arch

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "chybí curl i wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "chybí sha256sum i shasum, nejde ověřit stažený soubor"
	fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Stahuji $asset…"
fetch "$base/$asset" "$tmp/$asset" || fail "stažení $base/$asset selhalo"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || fail "stažení checksums.txt selhalo"

want=$(awk -v name="$asset" '$2 == name { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "$asset chybí v checksums.txt"
[ "$(sha256 "$tmp/$asset")" = "$want" ] || fail "kontrolní součet $asset nesedí, nic jsem neinstaloval"

mkdir -p "$dir"
chmod 755 "$tmp/$asset"
mv -f "$tmp/$asset" "$dir/housenka" || fail "nejde zapisovat do $dir (jiný adresář: HOUSENKA_INSTALL_DIR=…)"

echo "Nainstalováno: $dir/housenka ($("$dir/housenka" version))"

case ":$PATH:" in
*":$dir:"*) ;;
*)
	echo
	echo "$dir není v PATH. Přidej si do ~/.bashrc nebo ~/.zshrc:"
	echo "  export PATH=\"$dir:\$PATH\""
	;;
esac
