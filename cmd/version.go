package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"
	sthhttp "sth/internal/http"
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
		fmt.Sprintf("STH %s", valueOrDefault(version, "dev")),
		versionByline,
	}, "\n")
}

func (a *app) runVersion(args []string) error {
	flags, _ := parseFlags(args, "--json")
	if flags["--json"] {
		payload := struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
		}{
			Version: valueOrDefault(version, "dev"),
			Commit:  valueOrDefault(commit, "unknown"),
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		a.writeStdout(string(b))
		return nil
	}
	a.writeStdout(formatVersionSignature())
	return nil
}

func currentReleaseMetadata() sthhttp.ReleaseMetadata {
	assetName := strings.TrimSpace(releaseAssetName)
	if assetName == "" {
		assetName = defaultReleaseAssetName()
	}

	checksumName := strings.TrimSpace(releaseChecksumName)
	if checksumName == "" {
		checksumName = "SHA256SUMS"
	}

	return sthhttp.ReleaseMetadata{
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

	return fmt.Sprintf("sth-%s-%s%s", binaryOS, runtime.GOARCH, archiveExtension)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}