// Package fetch gère toutes les communications réseau de LoreSkills.
// Il est volontairement isolé de toute logique I/O fichier.
package fetch

import (
	"fmt"
	"io"
	"net/http"
)

// Download télécharge le contenu brut d'une URL distante et le retourne en octets.
// Retourne une erreur explicite si la réponse n'est pas HTTP 200
// (notamment HTTP 404 pour un skill inexistant).
func Download(url string) ([]byte, error) {
	// #nosec G107 — l'URL est construite depuis la config utilisateur, jamais codée en dur
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("échec de la requête vers %s : %w", url, err)
	}
	defer resp.Body.Close()

	// Cas 404 : le skill n'existe pas sur le dépôt distant
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("skill introuvable sur le dépôt distant (HTTP 404) : %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("réponse inattendue du serveur : HTTP %d pour %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lecture du corps de la réponse échouée : %w", err)
	}

	return data, nil
}
