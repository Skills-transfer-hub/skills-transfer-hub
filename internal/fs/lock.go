package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

const lockFileName = ".lore-state.lock"

// AcquireStateLock creates an advisory lock file in targetDir.
// It calls os.MkdirAll to ensure targetDir exists before creating the lock.
// Returns a release function that removes the lock file.
// Returns an error immediately if the lock cannot be acquired.
func AcquireStateLock(targetDir string) (func(), error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("création du répertoire cible échouée : %w", err)
	}
	lockPath := filepath.Join(filepath.Clean(targetDir), lockFileName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("fichier de verrouillage déjà présent (%s) — supprimez-le si aucun processus loreskills n'est en cours", lockPath)
		}
		return nil, fmt.Errorf("acquisition du verrou échouée : %w", err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("écriture du PID dans le fichier de verrouillage échouée : %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(lockPath)
		return nil, fmt.Errorf("fermeture du fichier de verrouillage échouée : %w", err)
	}
	return func() { os.Remove(lockPath) }, nil
}
