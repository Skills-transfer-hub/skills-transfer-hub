package cmd

import (
	"fmt"
	"os"
	"strings"
)

const experimentalEnvVar = "STH_EXPERIMENTAL"

func experimentalEnabled() bool {
	return strings.TrimSpace(os.Getenv(experimentalEnvVar)) == "1"
}

func errExperimentalDisabled(command string) error {
	return fmt.Errorf("commande indisponible : %q est en préversion et n'est pas encore activée", command)
}
