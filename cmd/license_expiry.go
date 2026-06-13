package cmd

import (
	"fmt"
	"time"

	"sth/internal/fs"
)

const licenseExpiryWarnWindow = 14 * 24 * time.Hour

type licenseExpiryState int

const (
	licenseExpiryUnknown licenseExpiryState = iota
	licenseExpiryValid
	licenseExpiryNear
	licenseExpiryExpired
)

func licenseExpiryStatus() (licenseExpiryState, time.Time) {
	settings, err := fs.ReadUserSettings()
	if err != nil || settings == nil || settings.Entitlements == nil {
		return licenseExpiryUnknown, time.Time{}
	}
	expiresAt := settings.Entitlements.ExpiresAt
	if expiresAt.IsZero() {
		return licenseExpiryUnknown, time.Time{}
	}
	now := time.Now().UTC()
	switch {
	case now.After(expiresAt):
		return licenseExpiryExpired, expiresAt
	case expiresAt.Sub(now) <= licenseExpiryWarnWindow:
		return licenseExpiryNear, expiresAt
	default:
		return licenseExpiryValid, expiresAt
	}
}

func licenseExpiryDaysRemaining(expiresAt time.Time) int {
	remaining := expiresAt.Sub(time.Now().UTC())
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Hours()/24) + 1
}

func licenseExpiryWarning() string {
	state, expiresAt := licenseExpiryStatus()
	switch state {
	case licenseExpiryExpired:
		return fmt.Sprintf(
			"⚠ Ta clé STH Cloud a expiré le %s — régénère-la sur le dashboard puis relance `sth login` avec la nouvelle clé.",
			expiresAt.Format("2006-01-02"),
		)
	case licenseExpiryNear:
		return fmt.Sprintf(
			"⚠ Ta clé STH Cloud expire dans %d jour(s) (le %s) — régénère-la sur le dashboard puis relance `sth login`.",
			licenseExpiryDaysRemaining(expiresAt), expiresAt.Format("2006-01-02"),
		)
	default:
		return ""
	}
}
