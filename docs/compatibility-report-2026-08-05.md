# Rapport de compatibilité STH — 5 août 2026

## Verdict

- **Implémentation STH : 20 contrats sur 20 OK.** Claude Code, Codex, GitHub Copilot, Antigravity et Gemini couvrent chacun skill, agent, commande et instruction.
- **Synchronisation réelle STH : OK.** Un binaire fraîchement compilé exécute `install`, `update --dry-run`, `update`, `status` et `remove` sur les vingt contrats contre un fournisseur GitHub local isolé.
- **Build et tests locaux : OK.** Build, vet, tests complets, race ciblée, compilation de test Windows, contrôle de diff et lifecycle réel sont verts.
- **Runner strict hermétique : OK, sortie 0 et matrice 38/38.** Il utilise un vrai binaire STH pour produire les fixtures `UPDATED`, lie leur provenance au SHA de ce binaire, à un challenge aléatoire et à un horodatage valable six heures, puis valide les cinq faux CLI et les preuves IDE Copilot et Antigravity. Chaque ligne atteste aussi le mode `hermetic` ou `real`.
- **Préparation stricte réelle renforcée : OK dans le harness.** Les cinq CLI doivent désormais fournir une version attendue et le SHA-256 attendu de leur exécutable résolu; les valeurs absentes ou divergentes sortent `2` avant le premier appel runtime. La preuve positive et les échecs version/SHA sont testés, sans modifier le strict hermétique ni le mode normal.
- **Runtime réel Codex : OK, 7 preuves sur 7.** Codex `0.147.0-alpha.1.2`, authentifié via ChatGPT, a validé trois découvertes et quatre sentinelles modèle réelles sur les fixtures `UPDATED`.
- **Runtime réel Copilot CLI : OK, 6 preuves `OK` et une inspection `INFO` autorisée.** Copilot `1.0.78` a validé les découvertes de skill et de commande puis les quatre sentinelles modèle réelles. Il a utilisé le jeton déjà détenu par `gh`, injecté uniquement dans l'environnement du processus; une comparaison après exécution trouve zéro copie de ce jeton dans le profil Copilot temporaire.
- **Runtime réel Antigravity : OK, CLI 5/5 et IDE 4/4.** Antigravity CLI `1.1.10` authentifié a validé sa découverte d'agent et les quatre sentinelles modèle réelles; l'IDE 2.1.1 a validé indépendamment workflow, commande, règle et instruction.
- **Runtime réel Antigravity IDE : OK, 4 preuves sur 4.** L'app 2.1.1 connectée a chargé le workflow et la règle des fixtures `UPDATED`, puis a retourné les deux sentinelles modèles exactes dans son `transcript_full.jsonl` natif. Le vérificateur accepte désormais le schéma natif 2.1.1 : requête encapsulée dans `USER_REQUEST`, événements sans `content` et préambule de tool-call non textuel. Cette preuve a été obtenue sur un challenge désormais expiré; elle devra être rejouée sur la racine de fixtures courante.
- **Runtime réel Claude : OK, 8 preuves sur 8.** Après déverrouillage manuel de `login.keychain-db`, `auth status` retourne `loggedIn: true` via `claude.ai`. Claude Code `2.1.201` a validé ses trois découvertes `system/init`, son `InstructionsLoaded`, puis les quatre sentinelles `CLAUDE_*_UPDATED` sur les fixtures courantes.
- **Runtime réel Copilot IDE classique : OK, 2 preuves sur 2.** VS Code `1.132.0` avec l'extension intégrée `GitHub.copilot-chat` `0.60.0` a résolu `.github/prompts/sth-runtime-command.prompt.md` en fichier de prompt racine, porté le challenge frais dans la requête et retourné `COPILOT_COMMAND_UPDATED`. Aucune preuve Copilot Agent Host n'est utilisée pour ce contrat.
- **Certification globale des runtimes externes réels : NON ACQUISE.** Aucune clé API ni projet Google n'étant disponible, les quatre smokes Gemini restent `NON_EXECUTE`, et la matrice stricte de 38 contrats ne peut pas sortir `0`.
- **CI distante : verte sur la tête distante précédente de la draft PR #185.** Les correctifs de vérificateurs de ce commit doivent encore être observés dans ce même CI gate.

Une découverte `OK` prouve que le consommateur a chargé l'artefact. Seule une sentinelle runtime `OK` issue de la réponse du modèle prouve son exécution. Une preuve hermétique valide le mécanisme de certification, pas le service externe.

## Matrice des vingt contrats

| Consommateur | Artefact | Destination produite par STH | Cycle STH réel | Preuve runtime au 5 août |
|---|---|---|---|---|
| Claude | Skill | `.claude/skills/<name>/SKILL.md` | OK | découverte `system/init` fraîche et smoke modèle réel 2.1.201 OK |
| Claude | Agent | `.claude/agents/<name>.md` | OK | découverte `system/init` fraîche et smoke modèle réel 2.1.201 OK |
| Claude | Commande | `.claude/commands/<name>.md` | OK | découverte `system/init` fraîche et smoke modèle réel 2.1.201 OK |
| Claude | Instruction | `.claude/rules/<name>.md` | OK | `InstructionsLoaded` frais et smoke modèle réel 2.1.201 OK |
| Codex | Skill | `.agents/skills/<name>/SKILL.md` | OK | découverte fraîche et smoke modèle réel 0.147 OK |
| Codex | Agent | `.codex/agents/<name>.toml` | OK | smoke modèle réel et délégation 0.147 OK |
| Codex | Commande | `.agents/skills/<name>/SKILL.md` | OK | découverte fraîche et smoke modèle réel 0.147 OK |
| Codex | Instruction | `.codex/sth/instructions/<name>.md` + région `AGENTS.md` | OK | découverte fraîche et smoke modèle réel 0.147 OK |
| Copilot | Skill | `.claude/skills/<name>/SKILL.md` | OK | découverte CLI fraîche et smoke modèle réel 1.0.78 OK |
| Copilot | Agent | `.github/agents/<name>.agent.md` | OK | smoke modèle réel 1.0.78 OK |
| Copilot | Commande | `.claude/commands/<name>.md` + `.github/prompts/<name>.prompt.md` | OK | découverte CLI fraîche OK; transcript IDE réel `0.60.0` OK |
| Copilot | Instruction | `.github/instructions/<name>.instructions.md` | OK | inspection optionnelle indisponible en CLI 1.0.78; smoke modèle réel OK |
| Antigravity | Skill | `.agents/skills/<name>/SKILL.md` | OK | smoke CLI réel 1.1.10 OK |
| Antigravity | Agent | `.agents/agents/<name>.md` | OK | découverte et smoke CLI réels 1.1.10 OK |
| Antigravity | Commande | `.agents/workflows/<name>.md` | OK | smoke CLI réel et preuve IDE réelle 2.1.1 OK |
| Antigravity | Instruction | `.agents/rules/<name>.md` | OK | smoke CLI réel et preuve IDE réelle 2.1.1 OK |
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
| Claude Code | `2.1.201` | `login.keychain-db` déverrouillé manuellement par l'utilisateur, ce qui lève l'erreur Keychain `-25293`; `auth status` retourne `loggedIn: true` via `claude.ai`; trois découvertes `system/init`, `InstructionsLoaded` et quatre smokes modèle réels OK, soit 8/8 |
| Codex | `0.147.0-alpha.1.2` | authentifié via ChatGPT; trois découvertes et quatre smokes modèle réels OK, soit 7/7 |
| GitHub Copilot CLI | `1.0.78` | installation temporaire; skill et commande découverts; quatre smokes modèle réels OK avec le jeton `gh` transmis uniquement en mémoire du processus; aucune copie du jeton détectée dans le profil temporaire après exécution |
| VS Code / Copilot intégré | `1.132.0` / `0.60.0` | trousseau déverrouillé, connexion GitHub aboutie dans le profil isolé; `GitHub.copilot-chat` est désormais une extension intégrée au bundle VS Code; transcript réel frais obtenu et validé |
| Gemini CLI | `0.53.1` | skill découvert; OAuth individuel refusé par le service; aucune clé ni projet configuré |
| Antigravity CLI | `1.1.10` | OAuth persistant vérifié par `antigravity models`; agent découvert avec `--add-dir` et quatre smokes modèle `UPDATED` réels OK |
| Antigravity IDE | `2.1.1` | preuve app `ANTIGRAVITY_IDE_APP_OK` relancée hors sandbox macOS et transcript natif frais `ANTIGRAVITY_IDE_OK` : workflow, commande, règle et instruction retournent les quatre preuves strictes attendues |

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

Le cumul indépendant actuellement acquis est de 34 clés sur 38 : ces douze découvertes ou inspections, dont l'`INFO` Copilot autorisé par la matrice, plus les quatre smokes modèle Claude, les quatre smokes modèle Codex, les quatre smokes modèle Copilot CLI, les quatre smokes modèle Antigravity CLI, les quatre preuves Antigravity IDE et les deux preuves Copilot IDE. Les quatre clés restantes sont les smokes modèle Gemini. Ce décompte rapproche des exécutions ciblées distinctes, menées sur plusieurs racines de fixtures successives; il ne constitue pas une sortie stricte unique, ne fusionne pas leurs TSV et ne certifie pas la matrice réelle globale. Sur la racine de fixtures courante, seules les huit clés Claude et les deux clés Copilot IDE ont été rejouées.

L'exécution ciblée réelle Codex est conservée dans `/private/tmp/sth-cert-final-20260805.Is2iWu/codex-real.tsv`. Ses sept lignes Codex portent toutes le mode `real` : `skill-discovery`, `command-discovery`, `instruction-discovery`, puis `skill`, `agent`, `command` et `instruction` avec les quatre sentinelles `UPDATED` exactes. Les autres consommateurs y sont volontairement absents ou `NON_EXECUTE`; ce fichier certifie Codex, pas la matrice externe globale.

La passe combinée Codex et Copilot CLI est conservée dans `/private/tmp/sth-cert-real-20260805.YphY1Z/normal-codex-copilot.tsv`. Elle contient treize lignes `OK`, une ligne `INFO` Copilot autorisée et dix-sept `NON_EXECUTE` volontairement neutralisés. Les sept lignes Codex sont toutes `OK`; Copilot fournit `skill-discovery`, `command-discovery` et les quatre sentinelles `COPILOT_*_UPDATED` en `OK`, plus `instruction-discovery` en `INFO`. Le jeton retourné par `gh auth token` n'a jamais été affiché ni écrit par le harness; la comparaison exacte de sa valeur avec tous les fichiers du profil temporaire après les quatre appels renvoie `COPILOT_RUNNER_TOKEN_PERSISTED_MATCHES=0`.

L'exécution ciblée Claude est conservée dans `/private/tmp/sth-cert-38-20260805/claude-real.tsv`. Ses huit lignes Claude portent toutes le mode `real` et le statut `OK` : `skill-discovery`, `agent-discovery`, `command-discovery` et `instruction-discovery`, puis `skill`, `agent`, `command` et `instruction` avec les quatre sentinelles `CLAUDE_*_UPDATED` exactes. Le blocage précédent tenait au verrouillage de `login.keychain-db`, que `security show-keychain-info` signalait par une erreur d'authentification; son déverrouillage manuel par l'utilisateur a rétabli la persistance d'authentification et rendu `auth status` positif. Aucun secret n'a été saisi, affiché ni écrit par le harness.

La preuve Copilot IDE classique réelle est conservée dans `/private/tmp/sth-cert-38-20260805/proofs/copilot-ide-transcript.jsonl`. Le transcript relie `GitHub.copilot-chat` version `0.60.0`, le fichier `.github/prompts/sth-runtime-command.prompt.md` en variable `promptFile` racine ajoutée automatiquement, le challenge courant présent dans le message, et une réponse modèle égale à `COPILOT_COMMAND_UPDATED`, sans `errorDetails`. `scripts/compat-copilot-ide-proof.sh` y répond `DISCOVERY_OK_RUNTIME_OK`.

Gemini CLI `0.53.1` refuse le flux OAuth individuel avec le message exact suivant :

```text
This client is no longer supported for Gemini Code Assist for individuals. To continue using Gemini, please migrate to the Antigravity suite of products: https://antigravity.google
```

Aucune clé API ni aucun projet Google Cloud n'étant configuré, les quatre smokes Gemini restent `NON_EXECUTE`. Le chemin réel a néanmoins été relancé avec le binaire 0.53.1 et la politique sécurisée du runner : la découverte du skill est `OK`, le premier smoke est classé `NON_EXECUTE` pour authentification absente, puis les trois suivants sont court-circuités. La version 0.53.1 ignore `--admin-policy` lorsqu'une policy système existe; le runner détecte désormais ce cas avant tout appel modèle et produit quatre `NON_EXECUTE` explicites. Ce blocage d'authentification externe ne remet pas en cause les contrats STH ni la preuve hermétique.

Antigravity CLI 1.1.10 ne charge l'agent de la fixture qu'avec `--add-dir <racine-absolue>`. Après l'échange OAuth, `antigravity models` liste les modèles disponibles et la passe `/private/tmp/sth-cert-real-20260805.YphY1Z/real-antigravity.tsv` confirme l'agent ainsi que les quatre sentinelles `ANTIGRAVITY_*_UPDATED`. Le runner exige également `status=SUCCESS`, une réponse JSON exacte et `--agent sth-runtime-agent` pour le contrat agent.

La passe réelle isolée Antigravity IDE est conservée dans `/private/tmp/sth-cert-real-20260805.YphY1Z/normal-antigravity-ide.tsv`. Ses quatre lignes `real` sont toutes `OK` : `command-discovery`, `command`, `instruction-discovery` et `instruction`. Le transcript natif réel comporte des événements système ou tool-call sans champ `content` et enveloppe la requête utilisateur dans `USER_REQUEST`; le vérificateur n'accepte ces variantes que lorsque le prompt exact est dans ce bloc, puis exige une unique réponse textuelle `MODEL/PLANNER_RESPONSE/DONE` égale à la sentinelle. Les tests hermétiques couvrent ce format ainsi que le refus d'un `content: null` et d'un prompt placé seulement dans les métadonnées.

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
COPILOT_IDE_EXTENSION_VERSION=0.60.0 \
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

`scripts/compat-copilot-ide-proof.sh` reconstruit d'abord la session à partir du journal VS Code, qui n'écrit plus un instantané complet mais un instantané `kind:0` suivi d'affectations `kind:1` et d'ajouts `kind:2`; une mise à jour sans instantané initial, un chemin introuvable ou un ajout sur une valeur non séquentielle sont refusés. Il vérifie ensuite que la dernière invocation associe la requête au fichier exact `.github/prompts/sth-runtime-command.prompt.md`, à l'extension `github.copilot-chat` dans la version attendue et au challenge frais, puis n'accepte la sentinelle que dans la réponse du modèle.

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
- correction du filtre de version d'identité runtime, qui rejetait une version suivie d'un point de ponctuation comme `GitHub Copilot CLI 1.0.78.`; la frontière de fin refuse toujours `1.2.30`, `1.2.3.4` et `1.2.3-beta`, et quatre tests hermétiques couvrent ces cas;
- lecture du journal de session incrémental de VS Code par la preuve Copilot IDE : instantané `kind:0`, affectation `kind:1` et ajout `kind:2` sont rejoués avant analyse, la réponse modèle étant lue à la fois sous `result.response` et sous `response`; l'épinglage d'extension, la résolution du fichier de prompt racine, le challenge frais et la dernière invocation restent exigés, et quatre tests hermétiques refusent une version d'extension divergente, une mise à jour sans instantané initial, un ajout sur `null` et tout `extensionId` autre que `github.copilot-chat`;
- job CI hermétique exact, avec téléchargement préalable des modules et gate agrégé.

## Conditions restantes pour 100 % runtime

1. Fournir à Gemini une méthode d'authentification encore supportée et le projet ou la clé requis, puis relancer ses quatre smokes; le flux OAuth individuel refusé ne peut pas servir de certification. Ce sont les seuls contrats encore dépourvus de toute preuve réelle.
2. Rejouer la session Antigravity IDE sur la racine de fixtures courante, le challenge de la preuve acquise ayant expiré.
3. Exécuter le strict réel en une passe unique, avec les cinq binaires et les deux transcripts IDE liés au même challenge, et obtenir la sortie `0` sur les 38 preuves exactes.
4. Publier les correctifs de vérificateurs sur la draft PR #185 vers `develop`, puis observer le `CI gate` GitHub vert depuis ce commit distant.

Tant que ces étapes externes ne sont pas terminées, le verdict exact reste : **STH, son lifecycle et son certificateur strict hermétique sont certifiés localement; le runtime multi-consommateur réel n'est pas certifié en une passe stricte unique, et la CI distante ne l'est pas encore pour ce commit**.

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
