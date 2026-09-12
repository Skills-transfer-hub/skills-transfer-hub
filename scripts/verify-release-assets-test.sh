#!/bin/sh
set -eu

script_directory=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repository_directory=$(dirname "$script_directory")
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/sth-release-assets-test.XXXXXX")
cleanup() {
	rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

asset_directory="$temporary_directory/assets"
mkdir -p "$asset_directory"
(cd "$repository_directory" && go build -trimpath -o "$temporary_directory/sth" .)

for engine_name in \
	sth-engine-linux-amd64 \
	sth-engine-darwin-amd64 \
	sth-engine-darwin-arm64 \
	sth-engine-windows-amd64.exe
do
	cp "$temporary_directory/sth" "$asset_directory/$engine_name"
done

create_tar_archive() {
	archive_name=$1
	staging_name=$2
	staging_directory="$temporary_directory/$staging_name"
	mkdir -p "$staging_directory"
	cp "$temporary_directory/sth" "$staging_directory/sth"
	cp "$repository_directory/LICENSE" "$staging_directory/LICENSE"
	cp "$repository_directory/README.md" "$staging_directory/README.md"
	(cd "$staging_directory" && tar -czf "$asset_directory/$archive_name" sth LICENSE README.md)
}

create_tar_archive sth-linux-amd64.tar.gz linux-amd64
create_tar_archive sth-darwin-amd64.tar.gz darwin-amd64
create_tar_archive sth-darwin-arm64.tar.gz darwin-arm64

windows_directory="$temporary_directory/windows-amd64"
mkdir -p "$windows_directory"
cp "$temporary_directory/sth" "$windows_directory/sth.exe"
cp "$repository_directory/LICENSE" "$windows_directory/LICENSE"
cp "$repository_directory/README.md" "$windows_directory/README.md"
(cd "$windows_directory" && zip -q "$asset_directory/sth-windows-amd64.zip" sth.exe LICENSE README.md)

sh "$script_directory/write-sth-code-engine-manifest.sh" v1.2.3 "$asset_directory/sth-code-engine.json"
sh "$script_directory/write-capabilities-manifest.sh" v1.2.3 "$asset_directory/sth-capabilities-v1.json"
sh "$script_directory/write-capabilities-documentation.sh" "$asset_directory/sth-capabilities-v1.json" "$asset_directory/sth-capabilities-v1.md"

expected_assets='sth-linux-amd64.tar.gz
sth-darwin-amd64.tar.gz
sth-darwin-arm64.tar.gz
sth-windows-amd64.zip
sth-engine-linux-amd64
sth-engine-darwin-amd64
sth-engine-darwin-arm64
sth-engine-windows-amd64.exe
sth-code-engine.json
sth-capabilities-v1.json
sth-capabilities-v1.json.sha256
sth-capabilities-v1.md'

: > "$asset_directory/SHA256SUMS"
for asset_name in $expected_assets; do
	if command -v sha256sum >/dev/null 2>&1; then
		(cd "$asset_directory" && sha256sum "$asset_name") >> "$asset_directory/SHA256SUMS"
	else
		(cd "$asset_directory" && shasum -a 256 "$asset_name") >> "$asset_directory/SHA256SUMS"
	fi
done

key_output=$(cd "$repository_directory" && go run ./tools/release-sign -genkey)
private_key=$(printf '%s\n' "$key_output" | sed -n 's/^STH_RELEASE_SIGNING_KEY=//p')
public_key=$(printf '%s\n' "$key_output" | sed -n 's/^releaseSigningPublicKeyBase64=//p')
if [ -z "$private_key" ] || [ -z "$public_key" ]; then
	printf 'unable to generate release signing fixture\n' >&2
	exit 1
fi
export STH_RELEASE_SIGNING_KEY=$private_key
export STH_RELEASE_SIGNING_PUBKEY=$public_key
(cd "$repository_directory" && go run ./tools/release-sign --in "$asset_directory/SHA256SUMS" --out "$asset_directory/SHA256SUMS.sig")

sh "$script_directory/verify-release-assets.sh" "$asset_directory" v1.2.3

goreleaser_directory="$asset_directory/goreleaser"
mkdir -p "$goreleaser_directory"
goreleaser_directory=$(CDPATH= cd -- "$goreleaser_directory" && pwd -P)
for engine_name in \
	sth-engine-linux-amd64 \
	sth-engine-darwin-amd64 \
	sth-engine-darwin-arm64 \
	sth-engine-windows-amd64.exe
do
	mv "$asset_directory/$engine_name" "$goreleaser_directory/$engine_name"
done

asset_directory_name=$(basename "$asset_directory")
{
	printf '[\n'
	first_artifact=1
	for engine_name in \
		sth-engine-linux-amd64 \
		sth-engine-darwin-amd64 \
		sth-engine-darwin-arm64 \
		sth-engine-windows-amd64.exe
	do
		if [ "$first_artifact" -eq 0 ]; then
			printf ',\n'
		fi
		printf '  {"name":"%s","path":"%s/goreleaser/%s"}' "$engine_name" "$asset_directory_name" "$engine_name"
		first_artifact=0
	done
	printf '\n]\n'
} > "$asset_directory/artifacts.json"

sh "$script_directory/verify-release-assets.sh" "$asset_directory" v1.2.3
resolved_linux_engine=$(sh "$script_directory/resolve-release-asset.sh" "$asset_directory" sth-engine-linux-amd64)
if [ "$resolved_linux_engine" != "$goreleaser_directory/sth-engine-linux-amd64" ]; then
	printf 'unexpected resolved release asset: %s\n' "$resolved_linux_engine" >&2
	exit 1
fi

cp "$asset_directory/artifacts.json" "$temporary_directory/artifacts.json"
cp "$goreleaser_directory/sth-engine-linux-amd64" "$temporary_directory/outside-engine"
jq 'map(if .name == "sth-engine-linux-amd64" then .path = "outside-engine" else . end)' "$temporary_directory/artifacts.json" > "$asset_directory/artifacts.json"
if sh "$script_directory/verify-release-assets.sh" "$asset_directory" v1.2.3 >/dev/null 2>&1; then
	printf 'release asset outside the asset directory was accepted\n' >&2
	exit 1
fi
cp "$temporary_directory/artifacts.json" "$asset_directory/artifacts.json"

mkdir -p "$temporary_directory/.release"
for metadata_name in sth-code-engine.json sth-capabilities-v1.json sth-capabilities-v1.json.sha256 sth-capabilities-v1.md; do
 mv "$asset_directory/$metadata_name" "$temporary_directory/.release/$metadata_name"
done
sh "$script_directory/verify-release-assets.sh" "$asset_directory" v1.2.3
mv "$temporary_directory/.release" "$temporary_directory/metadata-outside"
ln -s "$temporary_directory/metadata-outside" "$temporary_directory/.release"
if sh "$script_directory/verify-release-assets.sh" "$asset_directory" v1.2.3 >/dev/null 2>&1; then
 printf 'symlinked metadata directory was accepted\n' >&2
 exit 1
fi
rm "$temporary_directory/.release"
mv "$temporary_directory/metadata-outside" "$temporary_directory/.release"

printf 'tampered\n' >> "$goreleaser_directory/sth-engine-linux-amd64"
if sh "$script_directory/verify-release-assets.sh" "$asset_directory" v1.2.3 >/dev/null 2>&1; then
	printf 'tampered release asset was accepted\n' >&2
	exit 1
fi

printf 'release asset verifier tests passed\n'
