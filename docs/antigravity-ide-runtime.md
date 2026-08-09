# Validation Antigravity IDE

Le helper `scripts/compat-antigravity-ide-launch.sh` ouvre la fixture Antigravity produite par le vrai binaire STH, affiche les deux requêtes exactes à exécuter, attend la fermeture de la fenêtre puis valide automatiquement le transcript natif complet. Il n'automatise ni ne termine aucun OAuth.

## Préconditions

La racine de fixtures doit avoir été exportée depuis moins de six heures par `scripts/compat-sth-lifecycle-test.sh`. Le helper vérifie sa provenance `UPDATED` contre le SHA-256 du même `COMPAT_STH_BIN`. Il vérifie aussi le bundle, la version, l'exécutable, la signature Google et Gatekeeper avec `scripts/compat-antigravity-ide-app-proof.sh`.

```bash
COMPAT_FIXTURE_ROOT=/abs/cert/fixtures \
COMPAT_STH_BIN=/abs/cert/sth \
ANTIGRAVITY_IDE_APP='/Applications/Antigravity IDE.app' \
ANTIGRAVITY_IDE_VERSION=2.1.1 \
sh scripts/compat-antigravity-ide-launch.sh
```

Le CLI utilisé est obligatoirement celui du bundle validé :

```text
Contents/Resources/app/bin/antigravity-ide
```

Le helper neutralise `VSCODE_IPC_HOOK_CLI`, crée `user-data` et `extensions` sous un `mktemp`, puis transmet `--new-window`, `--wait`, `--sync off` et `--use-inmemory-secretstorage` au CLI de l'app.

## Requêtes manuelles exactes

Dans la fenêtre ouverte, exécuter les deux requêtes affichées, dans cet ordre :

```text
/sth-runtime-command STH_RUNTIME_CHALLENGE_<challenge> ANTIGRAVITY_IDE_VERSION_<version>
Return the STH instruction sentinel. STH_RUNTIME_CHALLENGE_<challenge> ANTIGRAVITY_IDE_VERSION_<version>
```

Fermer ensuite cette fenêtre. Le helper exige exactement un fichier `brain/*/.system_generated/logs/transcript_full.jsonl` modifié depuis le début de la session. Il refuse une absence ou plusieurs candidats, enregistre le chemin dans `session-info.tsv`, puis exécute `scripts/compat-antigravity-ide-proof.sh`. La commande ne réussit que si cette preuve renvoie exactement `ANTIGRAVITY_IDE_OK`.

La racine de recherche est par défaut calculée avec `pathlib.Path.home() / ".gemini" / "antigravity-ide"`. `ANTIGRAVITY_IDE_DATA_ROOT=/abs/path` permet de chercher dans une autre racine déjà utilisée par le backend. Cet override change la recherche du helper; il ne reconfigure pas à lui seul le stockage interne d'Antigravity.

## Frontière d'isolation

Le profil de type VS Code et les extensions de cette fenêtre sont isolés dans la session temporaire. Le helper ne réassigne jamais `HOME`.

Le flag `--use-inmemory-secretstorage` est transmis au shell IDE, mais il ne constitue pas une preuve que le backend Antigravity conserve son OAuth ou ses jetons uniquement en mémoire. Selon le chemin d'authentification et la disponibilité du porte-clés, ces données peuvent rester dans le macOS Keychain ou sous `~/.gemini/antigravity-ide`. Les transcripts natifs peuvent également y être écrits. Le helper ne promet donc pas un stockage secret entièrement éphémère.

Le dossier de session `mktemp` est volontairement conservé. Il contient les arguments, les requêtes, le challenge, les chemins et les sorties de preuve. `session-info.tsv` enregistre aussi `compat_mode`, `fixture_proof=FIXTURE_ROOT_OK` et `application_proof=ANTIGRAVITY_IDE_APP_OK`, y compris en `--prepare-only`; `application_status=not-run` signifie alors uniquement que le CLI de l'app n'a pas été lancé. Ces champs attestent les contrôles locaux effectués pendant la préparation, pas une provenance éditeur ni un lien cryptographique avec un transcript. Le transcript reste dans la racine de données Antigravity. Ne committer aucun de ces fichiers et les supprimer après la collecte autorisée de la preuve.

## Préparation sans lancement

Pour valider la provenance, l'app, le CLI, les arguments et les deux requêtes sans ouvrir Antigravity IDE :

```bash
COMPAT_FIXTURE_ROOT=/abs/cert/fixtures \
COMPAT_STH_BIN=/abs/cert/sth \
ANTIGRAVITY_IDE_APP='/Applications/Antigravity IDE.app' \
ANTIGRAVITY_IDE_VERSION=2.1.1 \
sh scripts/compat-antigravity-ide-launch.sh --prepare-only
```

Ce mode crée uniquement les métadonnées et dossiers temporaires de préparation; il ne lance pas le CLI de l'app et ne cherche aucun transcript.
