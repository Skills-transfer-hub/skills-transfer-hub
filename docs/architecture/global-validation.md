# Validation de l’installation globale V1

Date : 25 octobre 2026. Implémentation intégrée dans le dépôt local, sans publication ni remplacement du binaire STH installé sur la machine.

## Résultat livré

Les commandes `install`, `list`, `status`, `update` et `remove` acceptent `--global`/`-g`, indépendamment du projet courant. L’installation prend une source GitHub directe, des sélections répétables de skills/agents, un sous-dossier, une référence épinglée, le dry-run et le JSON versionné. Le parcours interactif, l’aide et les options de complétion sont disponibles.

Les consommateurs sont séparés des destinations physiques. Les huit surfaces des six familles sont distribuées dans quatre racines autorisées. Le cycle de vie préserve les consommateurs et pins, gère les conflits et modifications locales, sauvegarde les remplacements forcés et préserve les ajouts étrangers. Les transactions disposent d’un verrou interprocessus, d’un journal et d’une restauration qui refuse d’écraser les changements postérieurs à une interruption.

Les sources réutilisent les identifiants, le transport borné, le catalogue et le scanner existants. Le chemin local existant est conservé. La matrice locale de capacités 6×6, dont Cursor, est conservée telle quelle depuis la branche cible `develop`. Les seuls changements de dépendances déplacent `x/sys` et `x/text`, déjà présents aux mêmes versions, de dépendances indirectes vers directes.

Guide utilisateur : [global-skills.md](../global-skills.md).

## Vérifications exécutées

| Vérification | Résultat |
| --- | --- |
| `go build ./...` | PASS |
| `go test ./internal/fs/... ./cmd/...` | PASS |
| `go test ./internal/writer/... ./internal/scanner/... ./internal/http/...` | PASS |
| `go test ./... -count=1` | PASS sur tous les packages après correction de l’isolation des tests Cloud |
| `go test -race ./internal/fs ./internal/http ./internal/writer ./cmd -run '^TestGlobal' -count=1` | PASS |
| `go vet ./...` | PASS |
| `go mod tidy -diff` | Aucun écart |
| `GOOS=linux GOARCH=amd64 go build ./...` | Compilation PASS, exécution Linux non réalisée |
| `GOOS=windows GOARCH=amd64 go build ./...` | Compilation PASS, exécution Windows non réalisée |
| Analyse AST des nouveaux fichiers Go globaux | Aucun commentaire Go ni déclaration à une lettre interdite |
| Contrôle de diff | PASS ; le README suit les fins de ligne LF de la branche cible après intégration |

Les tests automatiques utilisent exclusivement des serveurs HTTP et des homes/configurations/projets temporaires. Les tests `cmd` restent séquentiels. Les tests FS exécutent aussi de vrais sous-processus pour vérifier le verrou interprocessus et une interruption via `os.Exit(71)` après remplacement des fichiers, avant publication de l’état.

La suite étendue a révélé deux échecs préexistants liés à l’environnement dans `internal/cloud` : les tests d’entitlements lisaient le trousseau macOS réel, qui prime sur leur clé d’environnement factice. Leur code était identique dans `HEAD`. Un `TestMain` de ce package initialise désormais le mock mémoire déjà fourni par `go-keyring`. Les deux tests concernés puis la suite complète passent. Aucun comportement d’authentification en production n’a changé ; aucune valeur de credential n’est incluse dans ce rapport.

## Preuves TDD

Les tests RED ont précédé les implémentations correspondantes. Les rapports séparent ces échecs attendus des problèmes d’environnement, compilations transitoires entre chantiers et tests de régression déjà verts.

- [CLI : parsing, routage, cycle de vie, parcours interactif et réinstallations](global-cli-tdd.md).
- [État, conflits, transactions, restauration, concurrence et chemins](global-fs-tdd.md).
- [GitHub : source, SHA, découverte, sélection et contenu](global-source-tdd.md).
- [Consommateurs, destinations et réconciliation Cursor](global-registry-tdd.md).
- [Revue indépendante et tests d’acceptation](global-review.md).

La revue a notamment fait échouer puis corriger l’update indépendant en présence d’un autre skill modifié, la perte des lecteurs possibles dans le résultat, la sélection de plusieurs skills à des SHA différents et la restauration trop tardive après un accès réseau. Les tests vérifient aussi les snapshots complets du projet, y compris `.sth/last-operation.json`, les fichiers tiers, fichiers manquants/disparus en amont, arbres tronqués, liens, collisions et journaux falsifiés.

## Parcours Ponytail réel

Une compilation locale a exécuté les commandes suivantes contre les endpoints GitHub réels, dans un home temporaire dédié et deux répertoires sans projet STH :

1. `install https://github.com/DietrichGebert/ponytail --skill ponytail --all-agents --global --dry-run --json`.
2. La même installation sans dry-run : quatre placements, huit consommateurs.
3. Réinstallation depuis le second répertoire : `unchanged`, état conservé octet pour octet.
4. `list --global --json` puis `status --global --json`.
5. `update ponytail --global --dry-run --json`, puis update réel : `unchanged`.
6. `remove ponytail --global --dry-run --json`, puis suppression réelle depuis le second répertoire.

Toutes ces commandes ont réussi. Révision résolue : `09c434ab73ca43761c5d8b0c5d341926c2fba710`. La source sélectionnée est `skills/ponytail`. Le dry-run initial n’a créé aucun fichier d’agent ou de configuration ; les deux répertoires de travail sont restés vides. L’état final ne contient plus d’installation.

Un essai direct supplémentaire de `--path skills --skill '*'` a reçu **HTTP 403** de GitHub avant installation. Ce résultat réseau n’est pas présenté comme une réussite ni attribué sans preuve à une cause précise.

Le contenu réel a ensuite été récupéré par un clone Git public `--bare --depth=1`, sans checkout ni exécution de script. Un serveur HTTP local a servi les arbres et blobs de ce même SHA depuis les objets Git. Le binaire a réussi le dry-run, l’installation et la suppression des six dossiers suivants, avec quatre placements chacun : `ponytail`, `ponytail-audit`, `ponytail-debt`, `ponytail-gain`, `ponytail-help`, `ponytail-review`. Le dry-run conserve l’état et les projets restent inchangés. C’est une preuve sur le vrai contenu avec transport HTTP local, distincte du premier parcours contre GitHub.

Le binaire a également renvoyé le code `2` et une enveloppe globale pour `update self --global --json`.

## Runtimes et limites

Exécution des tests de fichiers : macOS. Linux et Windows sont compilés et leurs règles portables sont testées, sans exécution native de ces systèmes. Les comparaisons de permissions Windows portent sur l’attribut lecture seule ; le mode Git reste enregistré. La synchronisation de métadonnées de répertoire n’est pas certifiée sur Windows. Aucune simulation de coupure d’alimentation n’a été effectuée.

Codex `0.164.0-alpha.11.1` a réellement découvert un skill utilisateur depuis `~/.agents/skills` dans deux projets vides. L’ajout d’un skill projet homonyme produit deux entrées distinctes. Cette vérification utilisait une fixture directement écrite dans le home isolé, sans authentification ni requête de modèle : elle prouve la découverte, pas l’invocation de Ponytail.

Claude Code `2.1.293` a répondu à l’initialisation SDK isolée en mode `--bare`, mais ce mode n’a pas énuméré les fixtures. La découverte normale de Claude reste non vérifiée. Cursor, Copilot CLI/VS Code, Gemini et Antigravity ne sont pas certifiés en runtime. Les combinaisons multi-agents et leur invocation restent non vérifiées. Détails : [global-runtime-validation.md](global-runtime-validation.md).

## Ajustements explicites du DD

Le DD demande les six familles tout en réservant `--all-agents` aux surfaces certifiées, puis constate qu’aucune certification globale n’existe. La V1 sélectionne donc les huit **contrats documentés de distribution** et affiche systématiquement `discovery-unverified`. `Supported` ne signifie pas certification d’un runtime. Cette décision permet la commande cible sans inventer de preuve d’exécution.

Les manifestes actifs sont refusés conservativement par nom à l’intérieur d’un skill, y compris s’ils ne sont que des exemples. Les méthodes existantes d’écriture locale ne sont pas étendues au home : la transaction globale réutilise les utilitaires compatibles et ajoute les validations multi-racines et la durabilité qui lui sont propres.

Le DD original et le dossier `output/`, déjà non suivis au début, ont été préservés. Aucun script de skill n’a été exécuté ; aucun skill n’a été installé dans le véritable home.

## Intégration de la PR

La V1 a été reportée sur `develop` au commit `29d9aaf` avant ouverture de la PR. La branche cible avait avancé de 28 commits depuis la base de conception. Les conflits de README et d’aide CLI ont été résolus en conservant les nouvelles commandes, options locales et capacités multi-artifacts de la cible. La matrice locale 6×6 incluait déjà Cursor : les changements historiques 20→24 ont donc été retirés du diff final, sans revenir aux anciens contrats.

Les traces RED/GREEN ci-dessus conservent l’historique réel de développement. Les parcours GitHub/Ponytail et les observations runtime ont été effectués avant ce report ; ils n’ont pas été présentés comme de nouveaux essais live après intégration. Après intégration, `go build ./...`, `go test ./... -count=1`, le détecteur de courses sur tous les tests globaux, `go vet ./...`, `go mod tidy -diff`, les compilations Linux/Windows et `git diff --cached --check` ont tous réussi. Le package `cmd` complet a terminé en 32,796 s et ses tests globaux avec `-race` en 10,504 s. Le DD initial non suivi et `output/` restent exclus du commit.
