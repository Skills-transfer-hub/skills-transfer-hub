package cmd

import (
	"fmt"
	lorehttp "loreskills/internal/http"
	"runtime"
	"strings"
)

var (
	version            = "dev"
	commit             = "unknown"
	buildDate          = "unknown"
	target             = runtime.GOOS + "/" + runtime.GOARCH
	releaseAPIURL      = ""
	releaseAssetName   = ""
	releaseChecksumName = "SHA256SUMS"
)

const versionByline = "dev with 🤖 by Hugo MR and Jeremy, Lead Tech."

func formatVersionSignature() string {
	return strings.Join([]string{
		fmt.Sprintf("LoreSkills %s", valueOrDefault(version, "dev")),
		versionByline,
	}, "\n")
}

func (a *app) runVersion() error {
	a.writeStdout(formatVersionSignature())
	return nil
}

func currentReleaseMetadata() lorehttp.ReleaseMetadata {
	assetName := strings.TrimSpace(releaseAssetName)
	if assetName == "" {
		assetName = defaultReleaseAssetName()
	}

	checksumName := strings.TrimSpace(releaseChecksumName)
	if checksumName == "" {
		checksumName = "SHA256SUMS"
	}

	return lorehttp.ReleaseMetadata{
		CurrentVersion: valueOrDefault(version, "dev"),
		ReleasesAPIURL: strings.TrimSpace(releaseAPIURL),
		AssetName:      assetName,
		ChecksumName:   checksumName,
	}
}

func defaultReleaseAssetName() string {
	archiveExtension := ".tar.gz"
	binaryOS := runtime.GOOS
	if runtime.GOOS == "windows" {
		archiveExtension = ".zip"
	}

	return fmt.Sprintf("loreskills-%s-%s%s", binaryOS, runtime.GOARCH, archiveExtension)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}