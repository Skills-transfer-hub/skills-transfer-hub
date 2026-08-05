# Validation Copilot IDE isolée

Le helper ouvre la fixture Copilot exportée par le vrai binaire STH dans une instance VS Code distincte. Les données utilisateur et les extensions externes sont placées sous un `mktemp`, la synchronisation est désactivée et le helper demande à VS Code `--use-inmemory-secretstorage`. Il ne déclenche ni ne termine automatiquement aucun OAuth et n'écrit lui-même aucun secret.

Le flag de secret storage décrit l'isolation demandée au shell VS Code; il ne prouve pas à lui seul le comportement effectif de GitHub Copilot, du flux OAuth ou du macOS Keychain. L'absence d'écriture persistante et la disparition de l'authentification après fermeture doivent être vérifiées interactuellement avant de qualifier la session d'entièrement éphémère.

La fixture doit avoir été exportée depuis moins de six heures par `scripts/compat-sth-lifecycle-test.sh`. `COMPAT_STH_BIN` est obligatoire : le helper compare son SHA-256 à la provenance avant toute préparation. Lancer ensuite :

```bash
COMPAT_FIXTURE_ROOT=/abs/cert/fixtures \
COMPAT_STH_BIN=/abs/cert/sth \
sh scripts/compat-copilot-ide-launch.sh
```

Dans la fenêtre isolée, terminer manuellement la connexion GitHub Copilot, puis envoyer exactement la requête affichée par le helper :

```text
/sth-runtime-command STH_RUNTIME_CHALLENGE_<challenge>
```

Fermer cette fenêtre après la réponse. Grâce à `--wait`, le helper reprend alors la main et affiche le dernier `User/workspaceStorage/*/chatSessions/*.jsonl` assez récent pour la preuve. Le même chemin est enregistré dans `session-info.tsv` sous la clé `transcript` et peut être fourni à `COPILOT_IDE_TRANSCRIPT`.

Pour vérifier la préparation sans ouvrir VS Code :

```bash
COMPAT_FIXTURE_ROOT=/abs/cert/fixtures \
COMPAT_STH_BIN=/abs/cert/sth \
sh scripts/compat-copilot-ide-launch.sh --prepare-only
```

Le dossier de session est volontairement conservé pour permettre la collecte du transcript. Il peut contenir le prompt, la réponse, des journaux et des chemins locaux : ne pas le committer, copier uniquement la preuve nécessaire, puis supprimer cette session temporaire après validation. La suppression de ce profil temporaire ne constitue pas une preuve que GitHub Copilot ou le système n'ont conservé aucun état d'authentification ailleurs.
