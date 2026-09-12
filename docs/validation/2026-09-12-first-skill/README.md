# Candidate du premier skill — 12 septembre 2026

## État

Candidate **0.2.43-snapshot**, construite avec GoReleaser **2.17.1** depuis une copie Git propre du commit **74302f4**. Aucune release publiée et aucune fusion. Les quatre archives et leurs binaires moteur sont vérifiés. La signature emploie une clé éphémère de test, pas la clé de distribution de production.

La certification dans tous les assistants reste **incomplète**. Codex exécute les fixtures, Copilot exécute le skill dans un essai limité à la lecture. Claude n’a pas de session modèle valide ; Gemini et Cursor sont absents. Antigravity exécute maintenant le skill installé dans un essai en mode plan avec sandbox ; la réponse exacte est conservée dans `antigravity-skill.json`.

## Branche et préservation

- Départ : `codex/release`, `c9f214d`, nombreux changements locaux.
- Sauvegarde indépendante : patch binaire et archive des fichiers non suivis sous `/private/tmp/sth-before-update-20260912-*` ; stash `safety: local multi-artifact work before first-skill update 2026-09-12` conservé.
- Les neuf commits jusqu’à `origin/develop` / `abfba87` sont intégrés avant les corrections produit.
- Cinq conflits résolus en conservant les contrats multi-source et multi-artifact. `go build ./...` et `go test ./internal/fs/... ./cmd/...` passent après intégration.
- `40d6014` isole le travail préexistant restauré. Les commits suivants contiennent les changements du premier parcours et les corrections issues des vérifications réelles.
- Les audits locaux préexistants sous `docs/audits/` restent hors de la PR.

## Corrections et régressions

| Problème | Changement | Test |
|---|---|---|
| Installation et suppression demandent des noms différents | Nom court, `skills/nom`, ID canonique, source qualifiée et `--provider` | `TestRemoveAcceptsInstallationReferences`, `TestRemoveScannedBundleUsesSameReference`, `TestRemoveAmbiguousReferencePreservesAllCandidates` |
| Token réclamé pour un dépôt public | Accès anonyme accepté ; le diagnostic réemploie le client d’installation | `TestDoctorPublicRepositoryNeedsNoToken`, `TestDoctorUsesTheInstallationCredentials` |
| Échec suivi de « ready » | Échec d’init enregistré, badge « connexion non vérifiée » au prochain menu | `TestFailedInitLeavesConnectionUnverified` |
| Première installation technique | Dépôt public → description du skill → assistant → aperçu → confirmation → utilisation | `TestFirstSkillMenuFromEmptyFolder`, `TestFirstSkillCancelWritesNoSkillFiles` |
| Texte et JSON changent de contenu | Même catalogue ; `--installed` sélectionne le contenu local dans les deux formats | `TestCatalogTextAndJSONDescribeSameRemoteSkills` et tests JSON/lifecycle adaptés |
| Réinstallation et update trompeuses | `unchanged` / « Déjà à jour », aucun fichier ni état réécrit si inchangé | `TestUnchangedSkillPreservesFilesStateAndChosenAssistant` |
| Mise à jour étend les cibles | Reprise des assistants enregistrés pour chaque skill | Même test |
| Édition personnelle menacée | Refus avec action concrète, copie personnelle conservée | Même test et parcours réseau |
| Quota anonyme traité comme credential invalide | HTTP 403 avec quota à zéro / HTTP 429 ne déclenchent plus de demande de token | `TestAnonymousRateLimitDoesNotTriggerAuthenticationRecovery` |
| Skills sans catalogue décrits de façon générique | Description extraite du frontmatter de `SKILL.md`, partagée par le menu et les listes | `TestScannedSkillDescriptionAppearsInMenuTextAndJSON` |
| Erreur réseau accompagnée d’une demande d’authentification | Conseil d’authentification limité aux erreurs d’accès typées | `TestAccessAdviceDistinguishesNetworkAndAuthenticationFailures` et parcours anonyme |
| Archives GoReleaser : manifestes introuvables | Résolution contrôlée des quatre métadonnées `.release`, rejet des symlinks | `scripts/verify-release-assets-test.sh` et vrai snapshot |

## Vérifications exécutées

- `go build ./...`, `go vet ./...`, `go test ./...` : succès ; suite `cmd` relancée après la dernière finition du menu.
- `scripts/compat-runtime-test.sh` : succès, runner **hermétique**, pas une preuve modèle.
- `scripts/verify-release-assets-test.sh` : succès, y compris métadonnées de snapshot, symlink interdit, binaire extérieur et checksum altéré.
- GoReleaser : macOS ARM64, macOS AMD64, Linux AMD64, Windows AMD64. Les binaires Linux/Windows sont compilés et leurs archives vérifiées ; ils ne sont pas exécutés sur ce Mac.
- `scripts/verify-release-assets.sh` : 12 assets vérifiés, checksums/signature de test, manifestes, égalité des binaires moteur et des binaires contenus dans les archives.
- Extraction de l’archive macOS ARM64 : `sth version` retourne `0.2.43-snapshot` ; handshake moteur protocole 1 conforme.
- Binaire extrait : cycle réel sur **24 contrats** `install → update --dry-run → update → status → remove`, avec fournisseur HTTP local isolé.
- Terminal PTY réel, dossier vide : choix dépôt, skill avec description, Codex, aperçu ; absence du fichier confirmée avant validation ; installation ; instructions `$premier-skill` ; « Mes skills » ; suppression avec aperçu ; absence du fichier confirmée après suppression.
- Dépôt GitHub public `anthropics/skills`, skill `skills/brand-guidelines` : catalogue, diagnostic, aperçu, installation, réinstallation inchangée, update inchangée, refus d’écraser une édition personnelle, retour au contenu initial et suppression **sans réseau** passent. Cet essai utilise l’accès GitHub déjà connecté, sans affichage ni stockage du token.
- Essai **anonyme** réel après reset du quota : succès avec la candidate `74302f4`, tokens et configuration Git globale désactivés. Catalogue avec descriptions réelles, diagnostic, aperçu, installation, réinstallation/update inchangées, protection des éditions et suppression hors réseau sont vérifiés dans `public-github-anonymous.log`. Une connexion refusée conseille maintenant de vérifier le réseau, sans suggestion d’authentification.

## Preuves assistants et limites

`runtime.tsv` contient l’essai du binaire `6217a13`, version `0.2.43-snapshot`. Les changements suivants portent sur l’affichage du menu, les descriptions du catalogue et les conseils après une erreur réseau ; ils ne modifient pas les fichiers installés. Le cycle des 24 contrats a été rejoué avec la candidate `74302f4`. Les fixtures ont été générées par le vrai STH à partir du cycle complet, avec challenge renouvelé ; elles ne sont pas des fichiers copiés à la main.

| Assistant | Chargement | Exécution du skill |
|---|---|---|
| Codex CLI 0.153.4 | OK | OK, `CODEX_SKILL_UPDATED` ; agents/commandes/instructions passent aussi |
| Claude Code 2.1.223 | OK, événements natifs sans appel modèle | NON EXÉCUTÉ, authentification absente |
| Copilot CLI 1.0.83 installé temporairement | OK, liste native du skill | OK, `COPILOT_SKILL_UPDATED` |
| Antigravity CLI | Agent découvert, skill chargé lors de son invocation | OK, `ANTIGRAVITY_SKILL_UPDATED`, mode plan et sandbox |
| Gemini CLI | CLI absent | NON EXÉCUTÉ |
| Cursor IDE | IDE absent | NON EXÉCUTÉ |

L’essai Copilot utilise `--available-tools=skill,view --allow-tool=skill --no-custom-instructions --disable-builtin-mcps --no-remote --no-remote-export --no-auto-update --no-ask-user --max-ai-credits 30`. Aucun shell ni outil d’écriture disponible. Les instructions générales de la fixture sont désactivées pour isoler le skill. Une première tentative limitée à `view` a retourné la sentinelle de l’instruction globale et n’a pas été retenue comme preuve du skill.

Le nouvel essai Antigravity invoque `/sth-runtime-skill` avec `--mode plan --sandbox --print-timeout 45s`. Il retourne exactement `ANTIGRAVITY_SKILL_UPDATED` en un tour. Ce résultat complète les lignes historiques non exécutées de `runtime.tsv` pour le skill ; il ne certifie pas les autres artifacts Antigravity.

## Migration de scripts

`sth list --json` représente désormais le même catalogue que `sth list`. L’ancien contenu local reste disponible via `sth list --installed --json`, avec `sth list --installed` pour son équivalent texte. `doctor --json` renvoie les mêmes contrôles et prochaines actions que le texte. `remove --json` renvoie le résultat ou l’erreur structurée.

## À terminer avant certification complète

1. Connecter Claude, fournir Gemini et Cursor, puis exécuter les skills dans ces assistants.
2. Garder la PR en brouillon tant que ces preuves et la CI finale ne sont pas acquises. Ne pas présenter les tests de fichiers ou les builds croisés comme des essais natifs des assistants ou de Windows/Linux.
