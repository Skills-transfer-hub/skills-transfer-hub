package cmd

import "testing"

func TestDetectPackageManager(t *testing.T) {
	cases := []struct {
		name        string
		execPath    string
		wantName    string
		wantHintHas string
	}{
		{
			name:        "macOS Intel Homebrew",
			execPath:    "/usr/local/Cellar/sth/0.1.0/bin/sth",
			wantName:    "Homebrew",
			wantHintHas: "brew upgrade",
		},
		{
			name:        "macOS Apple Silicon Homebrew",
			execPath:    "/opt/homebrew/Cellar/sth/0.1.0/bin/sth",
			wantName:    "Homebrew",
			wantHintHas: "brew upgrade",
		},
		{
			name:        "Linuxbrew",
			execPath:    "/home/linuxbrew/.linuxbrew/Cellar/sth/0.1.0/bin/sth",
			wantName:    "Homebrew",
			wantHintHas: "brew upgrade",
		},
		{
			name:        "Scoop with backslashes",
			execPath:    `C:\Users\hugo\scoop\apps\sth\current\sth.exe`,
			wantName:    "Scoop",
			wantHintHas: "scoop update",
		},
		{
			name:        "Scoop with forward slashes (mixed input)",
			execPath:    "C:/Users/hugo/scoop/apps/sth/current/sth.exe",
			wantName:    "Scoop",
			wantHintHas: "scoop update",
		},
		{
			name:        "manual /tmp install",
			execPath:    "/tmp/sth",
			wantName:    "",
			wantHintHas: "",
		},
		{
			name:        "Windows Program Files manual",
			execPath:    `C:\Program Files\sth\sth.exe`,
			wantName:    "",
			wantHintHas: "",
		},
		{
			name:        "empty path",
			execPath:    "",
			wantName:    "",
			wantHintHas: "",
		},
		{
			name:     "case-insensitive Cellar",
			execPath: "/opt/Homebrew/CELLAR/sth/0.1.0/bin/sth",
			wantName: "Homebrew",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectPackageManager(tc.execPath)
			if got.name != tc.wantName {
				t.Fatalf("detectPackageManager(%q).name = %q, want %q", tc.execPath, got.name, tc.wantName)
			}
			if tc.wantHintHas != "" {
				if got.upgradeHint == "" {
					t.Fatalf("upgradeHint = empty, want substring %q", tc.wantHintHas)
				}
				if !contains(got.upgradeHint, tc.wantHintHas) {
					t.Fatalf("upgradeHint = %q, want substring %q", got.upgradeHint, tc.wantHintHas)
				}
			}
			if tc.wantName == "" && got.upgradeHint != "" {
				t.Fatalf("upgradeHint = %q, want empty when name is empty", got.upgradeHint)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
