# Rapport de compatibilité STH — 5 août 2026

## Verdict

- **Implémentation STH : 20 contrats sur 20 OK.** Claude Code, Codex, GitHub Copilot, Antigravity et Gemini couvrent chacun skill, agent, commande et instruction.
- **Synchronisation réelle STH : OK.** Un binaire fraîchement compilé exécute `install`, `update --dry-run`, `update`, `status` et `remove` sur les vingt contrats contre un fournisseur GitHub local isolé.
- **Build et tests locaux : OK.** Build, vet, tests complets, race ciblée, compilation de test Windows, contrôle de diff et lifecycle réel sont verts.
- **Runner strict hermétique : OK, sortie 0 et matrice 38/38.** Il utilise un vrai binaire STH pour produire les fixtures `UPDATED`, lie leur provenance au SHA de ce binaire, à un challenge aléatoire et à un horodatage valable six heures, puis valide les cinq faux CLI et les preuves IDE Copilot et Antigravity. Chaque ligne atteste aussi le mode `hermetic` ou `real`.
- **Préparation stricte réelle renforcée : OK dans le harness.** Les cinq CLI doivent désormais fournir une version attendue et le SHA-256 attendu de leur exécutable résolu; les valeurs absentes ou divergentes sortent `2` avant le premier appel runtime. La preuve positive et les échecs version/SHA sont testés, sans modifier le strict hermétique ni le mode normal.
- **Runtime réel Codex : OK, 7 preuves sur 7.** Codex `0.147.0-alpha.1.2`, authentifié via ChatGPT, a validé trois découvertes et quatre sentinelles modèle réelles sur les fixtures `UPDATED`.
- **Runtime réel Copilot CLI : OK, 6 preuves `OK` et une inspection `INFO` autorisée.** Copilot `1.0.78` a validé les découvertes de skill et de commande puis les quatre sentinelles modèle réelles. Il a utilisé le jeton déjà détenu par `gh`, injecté uniquement dans l'environnement du processus; une comparaison après exécution trouve zéro copie de ce jeton dans le profil Copilot temporaire.
- **Certification globale des runtimes externes réels : NON ACQUISE.** Claude reste non authentifié malgré l'affichage `Login successful`, Gemini attend encore une clé ou un ADC explicite, Antigravity attend l'échange final de son OAuth, et les deux IDE exigent une validation interactive avec transcripts frais.
- **CI distante : NON OBSERVÉE.** Le commit `0be0051` est poussé sur `codex/runtime-five-client-certification`, mais le workflow de cette branche ne se déclenche que sur `pull_request`; aucune PR n'est encore ouverte vers `develop`.

Une découverte `OK` prouve que le consommateur a chargé l'artefact. Seule une sentinelle runtime `OK` issue de la réponse du modèle prouve son exécution. Une preuve hermétique valide le mécanisme de certification, pas le service externe.

## Matrice des vingt contrats

| Consommateur | Artefact | Destination produite par STH | Cycle STH réel | Preuve runtime au 5 août |
|---|---|---|---|---|
| Claude | Skill | `.claude/skills/<name>/SKILL.md` | OK | découverte `system/init` fraîche OK; modèle `NON_EXECUTE` — auth false |
| Claude | Agent | `.claude/agents/<name>.md` | OK | découverte `system/init` fraîche OK; modèle `NON_EXECUTE` — auth false |
| Claude | Commande | `.claude/commands/<name>.md` | OK | découverte `system/init` fraîche OK; modèle `NON_EXECUTE` — auth false |
| Claude | Instruction | `.claude/rules/<name>.md` | OK | `InstructionsLoaded` frais OK; modèle `NON_EXECUTE` — auth false |
| Codex | Skill | `.agents/skills/<name>/SKILL.md` | OK | découverte fraîche et smoke modèle réel 0.147 OK |
| Codex | Agent | `.codex/agents/<name>.toml` | OK | smoke modèle réel et délégation 0.147 OK |
| Codex | Commande | `.agents/skills/<name>/SKILL.md` | OK | découverte fraîche et smoke modèle réel 0.147 OK |
| Codex | Instruction | `.codex/sth/instructions/<name>.md` + région `AGENTS.md` | OK | découverte fraîche et smoke modèle réel 0.147 OK |
| Copilot | Skill | `.claude/skills/<name>/SKILL.md` | OK | découverte CLI fraîche et smoke modèle réel 1.0.78 OK |
| Copilot | Agent | `.github/agents/<name>.agent.md` | OK | smoke modèle réel 1.0.78 OK |
| Copilot | Commande | `.claude/commands/<name>.md` + `.github/prompts/<name>.prompt.md` | OK | découverte CLI fraîche OK; transcript IDE frais requis |
| Copilot | Instruction | `.github/instructions/<name>.instructions.md` | OK | inspection optionnelle indisponible en CLI 1.0.78; smoke modèle réel OK |
| Antigravity | Skill | `.agents/skills/<name>/SKILL.md` | OK | modèle à relancer après OAuth |
| Antigravity | Agent | `.agents/agents/<name>.md` | OK | découverte CLI fraîche OK; modèle à relancer après OAuth |
| Antigravity | Commande | `.agents/workflows/<name>.md` | OK | smoke CLI et preuve IDE prêts; OAuth et transcript frais requis |
| Antigravity | Instruction | `.agents/rules/<name>.md` | OK | smoke CLI et preuve IDE prêts; OAuth et transcript frais requis |
| Gemini | Skill | `.agents/skills/<name>/SKILL.md` | OK | découverte CLI fraîche OK; modèle `NON_EXECUTE` — OAuth individuel refusé |
| Gemini | Agent | `.gemini/agents/<name>.md` | OK | modèle `NON_EXECUTE` — aucune clé ni projet configuré |
| Gemini | Commande | `.gemini/commands/<name>.toml` | OK | modèle `NON_EXECUTE` — aucune clé ni projet configuré |
| Gemini | Instruction | `.gemini/sth/instructions/<name>.md` + région `GEMINI.md` | OK | modèle `NON_EXECUTE` — aucune clé ni projet configuré |

## Preuves exécutées

### Build, tests et portabilité

```text
go build ./...                                                   OK
go vet ./...                                                     OK
go test ./... -count=1                                          OK
go test -race ./internal/frontmatter ./internal/writer ./internal/fs/... ./cmd -count=1 OK
GOOS=windows GOARCH=amd64 go test -c ./cmd                       OK
git diff --check                                                 OK
sh -n scripts/compat-*.sh et faux CLI                            OK
sh scripts/compat-runtime-test.sh                                OK — strict hermétique 38/38, sortie 0
sh scripts/compat-sth-lifecycle-test.sh                          OK — 20 contrats
scripts/compat-fixture-root-proof.sh                             OK — binaire et fixtures liés par SHA-256
copie clean-room des seuls fichiers committables                 OK — chaîne complète, strict 38/38 et lifecycle 20/20
```

Les tests Go utilisant `httptest`, le lifecycle et la race ont été relancés hors sandbox afin d'autoriser uniquement l'écoute loopback. Une seconde validation a copié dans `/private/tmp/sth-clean-room.ZobaOc` uniquement `git ls-files -co --exclude-standard`, sans `.git` ni fichier ignoré, puis a réussi modules, vet, build natif, tests complets, race, build Windows, strict hermétique et lifecycle. Elle a d'abord détecté que les deux fragments `.codex/sth/...` et `.gemini/sth/...` étaient exclus par la règle `.gitignore` trop large `sth`; les règles sont désormais ancrées sur les seuls binaires racine `/sth` et `/sth.exe`, puis la clean-room complète est passée. Le job `Compatibility hermetic` télécharge désormais explicitement les modules avant le runner `GOPROXY=off`, vérifie la syntaxe de tous les scripts, exécute la preuve stricte hermétique et le lifecycle du binaire contre un fournisseur local. Cette configuration est validée localement; aucune exécution GitHub Actions de ces changements n'est encore revendiquée.

### Synchronisation par le vrai binaire STH

`scripts/compat-sth-lifecycle-test.sh` :

1. utilise le `STH_BIN` fourni ou compile `sth` avec `GOPROXY=off`;
2. expose un fournisseur GitHub simulé uniquement sur `127.0.0.1`;
3. installe les quatre artefacts dans chacun des cinq consommateurs;
4. compare les vingt contrats et les vingt-trois fichiers physiques attendus;
5. prouve que `update --dry-run` ne mute rien;
6. passe toutes les ressources de V1 à V2;
7. vérifie les quatre identifiants de ressource uniques et leurs statuts `up-to-date` par cible;
8. exporte, si demandé, les fixtures `UPDATED`, leur provenance et un challenge IDE aléatoire;
9. supprime tout et exige un statut final `[]`.

Le test coupe les tokens, la télémétrie et les proxys externes. Le fichier Copilot `sth-runtime-instruction-target.txt` reste une donnée du harness, pas un fichier produit par STH.

### Runner runtime réel

| Runtime | Version | État local fraîchement vérifié |
|---|---:|---|
| Claude Code | `2.1.201` | trois découvertes `system/init` et `InstructionsLoaded` OK; quatre modèles `NON_EXECUTE`; `auth status` reste faux après `Login successful`; `doctor` confirme l'échec d'écriture du macOS Keychain `-25293`, et `login.keychain-db` reste inaccessible en lecture hors sandbox même pendant une session graphique déverrouillée |
| Codex | `0.147.0-alpha.1.2` | authentifié via ChatGPT; trois découvertes et quatre smokes modèle réels OK, soit 7/7 |
| GitHub Copilot CLI | `1.0.78` | installation temporaire; skill et commande découverts; quatre smokes modèle réels OK avec le jeton `gh` transmis uniquement en mémoire du processus; aucune copie du jeton détectée dans le profil temporaire après exécution |
| VS Code / Copilot intégré | `1.131.0` / `0.59.0` | l'interface affiche `Se connecter`, `Models, sign in to use Copilot` et l'erreur de porte-clés; aucun mode `--password-store=basic` n'est actif; transcript réel frais toujours requis |
| Gemini CLI | `0.53.1` | skill découvert; OAuth individuel refusé par le service; aucune clé ni projet configuré |
| Antigravity CLI | `1.1.10` | binaire temporaire; agent découvert avec `--add-dir`; OAuth réel encore en attente |
| Antigravity IDE | `2.1.1` | preuve `ANTIGRAVITY_IDE_APP_OK` relancée hors sandbox macOS le 5 août; le sandbox produit un faux négatif sur codesign/Gatekeeper; aucun `transcript_full.jsonl` certifiant disponible |

Les deux lanceurs guidés passent en mode `--prepare-only` contre les fixtures `UPDATED` et le binaire STH exact. Copilot prépare son profil isolé sans ouvrir VS Code. Antigravity valide en plus l'app 2.1.1 présente hors sandbox macOS avant de produire les deux requêtes; aucune fenêtre ni aucun OAuth n'est lancé dans ce mode. Cette observation `ANTIGRAVITY_IDE_APP_OK` n'est ni une preuve persistée de provenance éditeur ni un lien cryptographique avec un transcript.

Les douze preuves de découverte ou d'inspection fraîches, obtenues avec `RUN_MODEL_SMOKES=0`, sont :

```text
Claude          skill-discovery        OK
Claude          agent-discovery        OK
Claude          command-discovery      OK
Claude          instruction-discovery  OK
Codex           skill-discovery        OK
Codex           command-discovery      OK
Codex           instruction-discovery  OK
Copilot CLI     skill-discovery        OK
Copilot CLI     command-discovery      OK
Copilot CLI     instruction-discovery  INFO
Gemini          skill-discovery        OK
Antigravity CLI agent-discovery        OK
```

Le cumul indépendant actuellement acquis est donc de 20 clés sur 38 : ces douze découvertes ou inspections, dont l'`INFO` Copilot autorisé par la matrice, plus les quatre smokes modèle Codex et les quatre smokes modèle Copilot CLI. Dix-huit clés restent sans preuve réelle. Ce décompte rapproche des exécutions ciblées distinctes; il ne constitue pas une sortie stricte unique, ne fusionne pas leurs TSV et ne certifie pas la matrice réelle globale.

L'exécution ciblée réelle Codex est conservée dans `/private/tmp/sth-cert-final-20260805.Is2iWu/codex-real.tsv`. Ses sept lignes Codex portent toutes le mode `real` : `skill-discovery`, `command-discovery`, `instruction-discovery`, puis `skill`, `agent`, `command` et `instruction` avec les quatre sentinelles `UPDATED` exactes. Les autres consommateurs y sont volontairement absents ou `NON_EXECUTE`; ce fichier certifie Codex, pas la matrice externe globale.

La passe combinée Codex et Copilot CLI est conservée dans `/private/tmp/sth-cert-real-20260805.YphY1Z/normal-codex-copilot.tsv`. Elle contient treize lignes `OK`, une ligne `INFO` Copilot autorisée et dix-sept `NON_EXECUTE` volontairement neutralisés. Les sept lignes Codex sont toutes `OK`; Copilot fournit `skill-discovery`, `command-discovery` et les quatre sentinelles `COPILOT_*_UPDATED` en `OK`, plus `instruction-discovery` en `INFO`. Le jeton retourné par `gh auth token` n'a jamais été affiché ni écrit par le harness; la comparaison exacte de sa valeur avec tous les fichiers du profil temporaire après les quatre appels renvoie `COPILOT_RUNNER_TOKEN_PERSISTED_MATCHES=0`.

L'exécution ciblée Claude est conservée dans `/private/tmp/sth-cert-final-20260805.Is2iWu/claude-real.tsv`. Elle prouve trois découvertes `system/init` et le chargement d'instruction `InstructionsLoaded`, puis quatre smokes modèle `NON_EXECUTE`. Le flux OAuth a affiché `Login successful`, mais `auth status` reste faux; `doctor` rattache ce désaccord à l'impossibilité d'écrire dans le macOS Keychain, code `-25293`. Aucun succès modèle Claude n'est donc revendiqué.

Gemini CLI `0.53.1` refuse le flux OAuth individuel avec le message exact suivant :

```text
This client is no longer supported for Gemini Code Assist for individuals. To continue using Gemini, please migrate to the Antigravity suite of products: https://antigravity.google
```

Aucune clé API ni aucun projet Google Cloud n'étant configuré, les quatre smokes Gemini restent `NON_EXECUTE`. Le chemin réel a néanmoins été relancé avec le binaire 0.53.1 et la politique sécurisée du runner : la découverte du skill est `OK`, le premier smoke est classé `NON_EXECUTE` pour authentification absente, puis les trois suivants sont court-circuités. La version 0.53.1 ignore `--admin-policy` lorsqu'une policy système existe; le runner détecte désormais ce cas avant tout appel modèle et produit quatre `NON_EXECUTE` explicites. Ce blocage d'authentification externe ne remet pas en cause les contrats STH ni la preuve hermétique.

Antigravity CLI 1.1.10 ne charge l'agent de la fixture qu'avec `--add-dir <racine-absolue>`. Le runner exige également `status=SUCCESS`, une réponse JSON exacte et `--agent sth-runtime-agent` pour le contrat agent.

### Certification stricte hermétique

`scripts/compat-runtime-test.sh` construit un vrai binaire STH, exporte ses fixtures `UPDATED`, vérifie leur SHA via `COMPAT_STH_BIN`, puis exécute les cinq faux CLI de succès et deux transcripts IDE contrôlés. Il exige :

- exactement les 38 clés de `expected-strict-contracts.tsv`;
- aucune clé manquante, dupliquée ou inattendue;
- aucun `KO` ni `NON_EXECUTE`;
- quatre appels modèle Claude, quatre Copilot, quatre Gemini, quatre Antigravity, et cinq Codex à cause du retry agent contrôlé;
- le rejet d'une racine de fixtures associée à un autre binaire.
- le rejet d'un faux CLI utilisé directement, masqué derrière un symlink ou un hard-link, ou copié octet pour octet;
- en strict réel seulement, le rejet de toute version ou empreinte attendue absente, mal formée ou différente, et la réussite du contrôle sur les cinq chemins résolus avant le premier appel runtime;
- la dernière invocation de chaque preuve IDE, jamais une réussite plus ancienne;
- le challenge aléatoire et l'âge maximal de six heures de la provenance dans les requêtes IDE;
- la version Copilot intégrée au transcript et, par une preuve distincte de co-présence, la version, le bundle, le TeamIdentifier Google `EQHXZ8M8AV`, la signature et l'acceptation Gatekeeper de l'app Antigravity en mode réel.

Cette preuve sort `0`. Les faux CLI embarqués et leurs copies exactes sont refusés hors `COMPAT_HERMETIC=1`. Le TSV porte le mode sur chaque ligne afin qu'un résultat hermétique ne puisse pas être présenté comme réel sans laisser une attestation contradictoire.

## Utilisation du runner

Audit hermétique et audit réel sans modèle :

```bash
sh scripts/compat-runtime-test.sh
RUN_MODEL_SMOKES=0 sh scripts/compat-runtime.sh
```

Préparation obligatoire d'une certification stricte réelle :

```bash
go build -o /abs/cert/sth .
STH_BIN=/abs/cert/sth \
COMPAT_FIXTURE_EXPORT_ROOT=/abs/cert/fixtures \
sh scripts/compat-sth-lifecycle-test.sh
```

Après exécution des deux validations IDE dans cette racine :

```bash
CLAUDE_BIN=/abs/cli/claude \
CLAUDE_COMPAT_EXPECTED_VERSION=2.1.201 \
CLAUDE_COMPAT_EXPECTED_SHA256='<sha256-claude-resolu>' \
CODEX_BIN=/abs/cli/codex \
CODEX_COMPAT_EXPECTED_VERSION=0.147.0-alpha.1.2 \
CODEX_COMPAT_EXPECTED_SHA256='<sha256-codex-resolu>' \
COPILOT_BIN=/abs/cli/copilot \
COPILOT_COMPAT_EXPECTED_VERSION=1.0.78 \
COPILOT_COMPAT_EXPECTED_SHA256='<sha256-copilot-resolu>' \
GEMINI_BIN=/abs/cli/gemini \
GEMINI_COMPAT_EXPECTED_VERSION=0.53.1 \
GEMINI_COMPAT_EXPECTED_SHA256='<sha256-gemini-resolu>' \
ANTIGRAVITY_BIN=/abs/cli/agy \
ANTIGRAVITY_COMPAT_EXPECTED_VERSION=1.1.10 \
ANTIGRAVITY_COMPAT_EXPECTED_SHA256='<sha256-antigravity-resolu>' \
COMPAT_STH_BIN=/abs/cert/sth \
COMPAT_FIXTURE_ROOT=/abs/cert/fixtures \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
COMPAT_HERMETIC=0 \
COPILOT_IDE_TRANSCRIPT=/abs/copilot-transcript.jsonl \
COPILOT_IDE_EXTENSION_VERSION=0.59.0 \
ANTIGRAVITY_IDE_TRANSCRIPT=/abs/transcript_full.jsonl \
ANTIGRAVITY_IDE_VERSION=2.1.1 \
ANTIGRAVITY_IDE_APP='/Applications/Antigravity IDE.app' \
STRICT_RUNTIME_CERTIFICATION=1 \
sh scripts/compat-runtime.sh \
  >/abs/cert/runtime-results.tsv \
  2>/abs/cert/runtime-identities.log
```

Les cinq SHA-256 doivent être relevés et enregistrés indépendamment avant l'exécution; les marqueurs `<sha256-...>` sont volontairement invalides tant qu'ils ne sont pas remplacés par 64 chiffres hexadécimaux. Le runner résout les chemins canoniques, vérifie `--version`, compare le contenu exact du fichier avant et après cette commande, puis écrit sur stderr une ligne `Identité runtime vérifiée: <nom> version=<v> sha256=<hash>` par CLI. Le fichier `runtime-identities.log` doit être conservé avec le TSV. Cette preuve lie le fichier local résolu au moment du contrôle; elle ne prouve ni sa provenance éditeur, ni sa signature, ni l'interpréteur ou les dépendances transitives d'un lanceur script.

L'export affiche `Challenge runtime IDE: <challenge>` et enregistre `generated_at`. La provenance expire six heures après cet horodatage. La dernière invocation Copilot doit contenir `STH_RUNTIME_CHALLENGE_<challenge>`. Les deux requêtes Antigravity doivent contenir ce challenge et `ANTIGRAVITY_IDE_VERSION_<version>`. Les versions de l'exemple doivent être remplacées par celles réellement utilisées. Un transcript issu d'un export antérieur ne peut donc pas satisfaire le challenge courant.

Le mode normal accepte `NON_EXECUTE` et échoue sur `KO`. Le mode strict vérifie la matrice exacte de 38 contrats et échoue sur `KO`, `NON_EXECUTE`, ligne manquante, dupliquée ou inattendue. Seul `copilot-cli/instruction-discovery` accepte `INFO`. Une erreur de préparation sort `2`, une matrice exécutée mais non certifiante sort `1`, et la certification complète sort `0`.

`scripts/compat-copilot-ide-proof.sh` vérifie que la dernière invocation VS Code associe la requête au fichier exact `.github/prompts/sth-runtime-command.prompt.md`, à la version d'extension attendue et au challenge frais, puis n'accepte la sentinelle que dans le résultat du modèle.

`scripts/compat-antigravity-ide-proof.sh` exige le fichier natif complet `transcript_full.jsonl`, refuse les transcripts tronqués ou ambigus, valide la dernière invocation de chaque requête avec challenge, la rattache à une unique réponse `MODEL/PLANNER_RESPONSE/DONE`, puis valide séparément workflow et règle. `scripts/compat-antigravity-ide-app-proof.sh` exige séparément la co-présence du bundle `com.google.antigravity-ide` à la version annoncée, signé par le TeamIdentifier Google `EQHXZ8M8AV` et accepté par Gatekeeper. Le transcript natif n'expose pas d'identifiant de build permettant de relier cryptographiquement la réponse à cette app précise. Une sentinelle présente uniquement dans une fixture, un message système ou une ancienne réponse ne produit pas de faux positif.

`scripts/compat-copilot-ide-launch.sh` prépare un profil VS Code et un dossier d'extensions sous `mktemp`, désactive la synchronisation, demande `--use-inmemory-secretstorage` et collecte le transcript de cette session isolée. Ce flag ne prouve pas à lui seul où GitHub Copilot, OAuth ou le Keychain persistent réellement leurs données. `scripts/compat-antigravity-ide-launch.sh` applique la même isolation au shell IDE, vérifie d'abord la provenance et l'app, conserve `compat_mode`, `fixture_proof` et `application_proof` dans `session-info.tsv`, affiche les deux requêtes exactes, puis exige un unique `transcript_full.jsonl` frais et exécute automatiquement la preuve. Il ne réassigne jamais `HOME` et ne prétend pas isoler le stockage interne du backend Antigravity, qui peut encore utiliser le Keychain ou `~/.gemini/antigravity-ide`. Les procédures complètes sont dans `docs/copilot-ide-runtime.md` et `docs/antigravity-ide-runtime.md`.

## Corrections intégrées

- matrice explicite des vingt capacités, toutes implémentées;
- cycle réel transactionnel du binaire STH sur les cinq cibles et les quatre artefacts;
- validation native de chaque consommateur de skill avant déduplication : noms canoniques Agent Skills de 1 à 64 caractères identiques au dossier, descriptions non vides limitées à 1024 caractères, et variante Antigravity où `name` peut être omis mais le dossier reste canonique;
- rejet avant mutation des `root_dir` explicites non canoniques, au lieu de les ignorer silencieusement;
- parsing frontmatter YAML des scalaires simples, quotés, pliés et littéraux dans les formats Codex, Copilot et Gemini;
- parseur YAML partagé avec le scanner et les métadonnées de publication;
- agents Codex natifs en TOML, commandes adaptées en skills et instructions agrégées dans `AGENTS.md`;
- commandes Copilot produites pour la CLI et l'IDE, avec preuve de transcript IDE;
- instructions Gemini matérialisées en fragments importés par une région gérée de `GEMINI.md`;
- régions Markdown atomiques, idempotentes et transactionnelles;
- protection de l'import Gemini final sans retour à la ligne et restauration octet pour octet;
- règles `.gitignore` ancrées sur les binaires racine afin que les fragments générés `.codex/sth` et `.gemini/sth` soient réellement committables et présents dans un clone;
- ownership, collisions de casse, confinement de chemins et rollback;
- retry Codex borné à deux tentatives pour la délégation d'agent transitoire;
- séparation stdout/stderr pour les cinq smokes afin de ne valider que la sortie structurée du modèle;
- isolation Gemini sous un `GEMINI_CLI_HOME` et un `CLOUDSDK_CONFIG` temporaires, fallback ADC ambiant bloqué sauf chemin de credentials explicitement fourni, endpoint Code Assist officiel verrouillé, bases personnalisées neutralisées, tout `.gemini/.env` sur la chaîne de parents et tout dotenv du home isolé interdits, relance, sandbox, hooks, télémétrie et statistiques désactivés, extensions coupées et MCP limité à un nom aléatoire inexistant pendant les smokes;
- smokes Gemini en mode `plan` avec politique deny-all sauf le skill et l'agent testés, et court-circuit explicite si une policy système ferait ignorer cette politique supplémentaire;
- détection élargie des erreurs OAuth et journal natif Antigravity isolé;
- preuve IDE Antigravity native couvrant quatre contrats;
- lanceurs guidés Copilot IDE et Antigravity IDE avec profils temporaires, challenges frais, collecte de transcript et tests hermétiques;
- provenance des fixtures liée au binaire STH exact, à un challenge IDE aléatoire et à une fenêtre de fraîcheur de six heures;
- protections anti-replay sur la dernière invocation et validation de version des deux IDE;
- preuve stricte hermétique atteignable à 38/38, mode attesté dans le TSV et faux CLI exacts interdits en mode réel;
- identité des cinq CLI liée en strict réel à une version explicitement attendue et au SHA-256 exact du fichier résolu, avec attestation stderr et tests positifs/négatifs;
- job CI hermétique exact, avec téléchargement préalable des modules et gate agrégé.

## Conditions restantes pour 100 % runtime

1. Déverrouiller ou réparer manuellement `login.keychain-db`, rétablir une persistance d'authentification Claude malgré l'erreur Keychain `-25293`, puis confirmer `auth status` avant de relancer ses quatre modèles.
2. Terminer l'échange OAuth Antigravity avec le compte choisi, puis relancer ses quatre smokes CLI; les quatre smokes Copilot CLI sont déjà `OK` sans persistance supplémentaire.
3. Fournir à Gemini une méthode d'authentification encore supportée et le projet ou la clé requis, puis relancer ses quatre smokes; le flux OAuth individuel refusé ne peut pas servir de certification.
4. Résoudre la connexion et l'erreur de porte-clés de VS Code, exécuter `/sth-runtime-command` avec le challenge frais et valider la réponse exacte dans le transcript de la version Copilot annoncée.
5. Ouvrir la fixture générée dans l'app Antigravity signée connectée, exécuter le workflow et la règle avec le challenge frais, puis fournir le `transcript_full.jsonl` natif.
6. Exécuter le strict réel et obtenir la sortie `0` sur les 38 preuves exactes.
7. Ouvrir une PR vers `develop` depuis la branche déjà poussée, puis observer le `CI gate` GitHub vert depuis le commit distant.

Tant que ces étapes externes ne sont pas terminées, le verdict exact reste : **STH, son lifecycle et son certificateur strict hermétique sont certifiés localement; le runtime multi-consommateur réel et la CI distante ne le sont pas encore**.

## Références officielles

- [Claude Code CLI](https://code.claude.com/docs/en/cli-usage)
- [Claude Code hooks](https://code.claude.com/docs/en/hooks)
- [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
- [Codex skills](https://learn.chatgpt.com/docs/build-skills)
- [GitHub Copilot CLI](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference)
- [GitHub Copilot prompt files](https://code.visualstudio.com/docs/copilot/customization/prompt-files)
- [Antigravity CLI installation](https://antigravity.google/docs/cli/install)
- [Antigravity custom agents](https://antigravity.google/docs/cli/commands/agents)
- [Antigravity workflows](https://antigravity.google/docs/ide/workflows)
- [Antigravity rules](https://antigravity.google/docs/ide-rules)
- [Antigravity IDE hooks et transcripts](https://antigravity.google/docs/ide-hooks)
- [Gemini CLI installation](https://geminicli.com/docs/get-started/installation/)
- [Gemini CLI context imports](https://geminicli.com/docs/cli/gemini-md/)
- [Agent Skills specification](https://agentskills.io/specification)
