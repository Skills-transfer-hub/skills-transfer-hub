package cmd

import "sth/internal/tui"

func (a *app) runGuidedSources(session *tui.Session) error {
	if err := a.runProviderList(); err != nil {
		return err
	}
	selected, err := tui.Select(session, tui.SelectConfig{Title: "Mes dépôts", Options: []tui.Option{
		{Label: "Ajouter un dépôt public ou privé", Aliases: []string{"add", "ajouter"}},
		{Label: "Créer une clé pour un dépôt privé", Aliases: []string{"guide", "cle", "clé"}},
		{Label: "Vérifier les connexions", Aliases: []string{"diagnostic", "doctor"}},
		{Label: "Retour", Aliases: []string{"back", "retour", "quit"}},
	}})
	if err != nil {
		return err
	}
	switch selected {
	case 0:
		return a.runInit("--no-cloud-prompt")
	case 1:
		provider, err := tui.Select(session, tui.SelectConfig{Title: "Où se trouve le dépôt privé ?", Options: providerSelectOptions()[:4]})
		if err != nil {
			return err
		}
		a.writeStdout(privateCreationGuide(providerKeysOrdered[provider])...)
	case 2:
		return a.runDoctor(nil)
	}
	return nil
}

func privateCreationGuide(provider string) []string {
	guide, ok := findProviderGuide(provider)
	if !ok {
		return providerPrivateAccessGuidance(provider)
	}
	steps := []string{"Créer une clé de lecture pour " + guide.DisplayName, "1. Ouvrez " + guide.ActionURL, "2. Nommez la clé STH et choisissez une date d’expiration."}
	if provider == "github" {
		steps = append(steps, "3. Choisissez le propriétaire du dépôt, puis Only select repositories et le dépôt concerné.", "4. Repository permissions → Contents → Read-only. Si nécessaire, faites approuver la clé par l’organisation.")
	} else {
		steps = append(steps, "3. Accordez uniquement les permissions de lecture : "+guide.MinimumPermission+".")
		if provider == "gitlab" {
			steps = append(steps, "Le scope read_api permet aussi de lire l’arborescence et les versions ; read_repository seul ne couvre pas ces contrôles API.")
		}
	}
	steps = append(steps, "Générez la clé et copiez-la dans le champ masqué du guide de connexion.", "Pour les prochains lancements : configurez "+providerTokenEnvVar(provider)+" ou votre gestionnaire d’identifiants Git.", "Avec plusieurs providers, utilisez leurs variables dédiées plutôt que STH_TOKEN.", "La clé n’est pas enregistrée dans les fichiers du projet.", "Vérifiez ensuite la connexion dans Diagnostic.")
	return steps
}
