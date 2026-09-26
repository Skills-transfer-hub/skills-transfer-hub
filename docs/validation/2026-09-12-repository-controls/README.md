# Contrôle des dépôts et parcours multi-source

## Changements

- À l’ouverture du menu, les versions des skills installés sont vérifiées en arrière-plan. Le menu reste utilisable pendant le contrôle. Aucune mise à jour n’est appliquée automatiquement.
- Le cache dure cinq minutes dans la session. Un changement de configuration ou de version installée invalide le résultat. Diagnostic et la configuration des sources déclenchent un nouveau contrôle au retour au menu. Chaque requête a un délai de deux secondes, dans une enveloppe de huit secondes ; les résultats non obtenus restent explicitement non vérifiés.
- Le résumé distingue les mises à jour disponibles, les versions épinglées et les sources non vérifiées. `sth check-updates` et `sth check-updates --json` donnent le détail par source ; une source en échec ne masque pas les autres.
- « Mes dépôts » donne accès à l’ajout de sources publiques ou privées, au guide de création de clé et au diagnostic. Le guide est aussi affiché dans la configuration privée lorsqu’une clé existe déjà. Les instructions GitHub privilégient un dépôt sélectionné et `Contents: Read-only`. GitLab utilise `read_api`, nécessaire pour les contrôles de versions et d’arborescence, au-delà de la seule lecture de fichiers.
- Au-delà de vingt skills, le parcours propose une recherche par nom, description ou source. La sélection affiche huit entrées à la fois. Les descriptions des grands catalogues sont chargées avec huit lectures simultanées au maximum. Un fichier inaccessible ne masque pas les autres descriptions.
- Deux noms identiques ne sélectionnent plus silencieusement la première source. La référence qualifiée et la sélection explicite restent disponibles.
- Les agents Copilot `agents/nom.agent.md` sont maintenant nommés `nom`, en conservant leur chemin source. Le suffixe `.agent` ne provoque plus une erreur de nom invalide lors de l’installation.

## Vérifications

`go test ./...` et `go vet ./...` passent. Les tests ciblés sous `-race` passent pour le contrôle asynchrone et le chargement des descriptions. Les tests vérifient aussi le cache, son invalidation, les sources partiellement indisponibles, l’absence d’écriture par le contrôle automatique, la pagination, les noms ambigus et les identifiants distincts GitHub/GitLab.

Dans un vrai terminal, le menu apparaît avec « Vérification des mises à jour des skills… », puis annonce une mise à jour disponible sur le dépôt HTTP de test, sans action utilisateur. La touche `q` quitte normalement. Le contrôle est distinct de la recherche des versions du programme STH.

## Dépôts publics réels

Catalogue agrégé sans token, le 12 septembre 2026 : **699 ressources**, en environ **18 secondes**, sans description de skill inaccessible.

| Source | Fournisseur | Format observé | Catalogue |
|---|---|---|---|
| anthropics/skills | GitHub | Dossiers `skills/nom/SKILL.md` | 19 skills |
| github/awesome-copilot | GitHub | Skills et fichiers `agents/nom.agent.md` | 435 skills, 222 agents |
| gitlab-org/ai/skills | GitLab | Skills avec fichiers de référence | 23 skills |

Le cycle réel dans un seul projet temporaire passe pour `anthropic:skills/brand-guidelines`, `gitlab:skills/commit-messages` et `copilot:agents/api-architect` : aperçu, installation vers Codex, contrôle des versions en texte et JSON, puis suppression avec les mêmes références. La liste locale finale est vide.

L’agent Copilot est une conversion **lossy** vers Codex, annoncée dans l’aperçu et explicitement autorisée par `--allow exact,lossy`. Ce test valide le transfert et la gestion des fichiers ; il ne constitue pas un essai modèle supplémentaire.

Le catalogue et les deux premières installations de skills ont fonctionné anonymement. Le quota GitHub anonyme a ensuite été épuisé. Le cycle final complet utilise la session GitHub existante, uniquement via `GITHUB_TOKEN` en mémoire du processus ; GitLab reste anonyme. Le test complet ne doit pas être présenté comme entièrement anonyme.

## Dépôt privé réel

Sur un dépôt privé STH existant, la lecture d’un même `README.md` par le client de diagnostic échoue sans identifiant et réussit avec la session GitHub existante. Le dépôt n’a pas de catalogue de skills : ce résultat prouve la connexion privée, pas une installation de skill privé. Les contrôles privés GitHub/GitLab avec clés valides et invalides sont en plus couverts par serveur HTTP isolé. Aucun contenu du dépôt privé ni aucune clé n’est joint aux preuves.

## Sources des guides

- [GitHub : gestion des personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)
- [GitLab : scopes des access tokens](https://docs.gitlab.com/security/tokens/access_token_scopes/)

## Candidate finale

La candidate `0.2.43-snapshot` est reconstruite depuis le commit `93d8a99` dans un clone propre : quatre plateformes, douze assets, archives, checksums et signature vérifiés. La clé publique jointe correspond à une signature éphémère de test, pas à une publication de production. Le menu dégrade aussi son badge en « connexion non vérifiée » lorsqu’un contrôle automatique échoue ; le test du vrai modèle de menu couvre cette régression. Le guide GitHub a été parcouru dans un terminal interactif via « Mes dépôts ».

Le binaire macOS extrait de cette candidate passe les 24 contrats du parcours installation, mise à jour et suppression ; voir `candidate-lifecycle.log`.
