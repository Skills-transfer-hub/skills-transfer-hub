# Installer des skills pour l’utilisateur courant

La portée globale fonctionne depuis n’importe quel répertoire, sans `sth init` et sans compte STH pour un dépôt GitHub public. Elle installe des dossiers de skills dans les emplacements documentés des agents sur la machine d’exécution.

```sh
sth install https://github.com/DietrichGebert/ponytail --skill ponytail --global --all-agents
sth install github:owner/repo -g --agent claude --agent codex --skill review
sth install github:owner/repo -g --path skills --skill '*' --all-agents --dry-run --json
sth install github:owner/repo -g --path skills/review --ref v1.2.0 --agent codex
```

`--agent` et `--skill` sont répétables. Les références `--ref` peuvent être une branche, un tag ou un SHA. Les URL `tree/` et `blob/` ne sont pas acceptées : utiliser `--path` et `--ref`. Un nom de catalogue de projet n’est pas une source globale.

Sans sélection d’agents, une session interactive propose les surfaces disponibles et signale les exécutables détectés. Un dossier seul n’établit pas qu’un agent est installé. Hors terminal, fournir `--agent` ou `--all-agents`. Un seul skill découvert est sélectionné automatiquement ; plusieurs candidats exigent un choix. Un nom ambigu exige `--path`. `--yes` ne choisit rien à votre place et n’autorise aucun écrasement.

## Consommateurs et destinations

| Sélection | Destination de copie |
| --- | --- |
| `claude` | `~/.claude/skills/<nom>` |
| `codex`, `cursor`, `gemini`, `copilot-cli`, `copilot-vscode` | `~/.agents/skills/<nom>` |
| `antigravity-ide` | `~/.gemini/config/skills/<nom>` |
| `antigravity-cli` | `~/.gemini/antigravity-cli/skills/<nom>` |

`copilot` sélectionne ses deux surfaces ; `antigravity` sélectionne IDE/App et CLI. `--all-agents` sélectionne les huit surfaces supportées par le contrat de distribution livré, soit quatre copies physiques par skill. Les dossiers peuvent être créés même si l’agent n’est pas installé. Les capacités restent limitées aux skills.

Les chemins sont documentés et les copies testées par STH ; cela ne certifie pas leur chargement par tous les runtimes. Le résultat expose séparément les consommateurs demandés (`requested_consumers`), les destinations et leurs lecteurs possibles (`possible_readers`). Un hub partagé peut être lu par d’autres agents. Des copies homonymes dans les autres emplacements connus donnent lieu à un avertissement et restent intactes. Les combinaisons de lecteurs peuvent afficher des doublons.

Le résultat `files-installed` reste accompagné de `discovery-unverified`. Ouvrir une nouvelle session ou recharger les skills dans l’hôte. STH ne lance pas de modèle pour vérifier l’installation. Aucune priorité générale entre les skills du projet et ceux de l’utilisateur n’est présumée. Les vérifications réelles disponibles sont consignées dans [le rapport runtime](architecture/global-runtime-validation.md).

## Cycle de vie

```sh
sth list --global
sth status --global --json
sth update ponytail --global --dry-run
sth update --global
sth remove ponytail --global --dry-run
sth remove ponytail --global
```

`list` est un inventaire installé hors réseau, identique en texte et JSON. `status` vérifie les fichiers et permissions ; si le réseau échoue, le résultat local reste disponible et la révision distante est `unknown`. Les noms locaux uniques et les IDs canoniques sont acceptés : `github:github.com/owner/repo#skills/review`.

Une réinstallation ajoute les consommateurs demandés et garde les précédents. Sans `--ref`, chaque installation existante conserve son SHA, même si la branche a avancé ou si plusieurs skills du dépôt ont des révisions différentes. Les nouveaux skills sont résolus depuis la branche courante. `update` conserve la liste des consommateurs enregistrée ; il n’ajoute pas de nouvelles surfaces apparues dans une version ultérieure de STH.

Sans `--ref` au premier install, les updates suivent la branche par défaut enregistrée à ce moment. Avec `--ref`, le SHA résolu est épinglé : un tag ou une branche déplacés ne font pas avancer `update`. Une réinstallation avec un `--ref` explicite remplace le pin et toutes ses copies dans la même transaction. `update` répare les fichiers manquants et retire les anciens fichiers suivis supprimés en amont.

`sth update self --global` est refusé. La mise à jour du binaire reste `sth update self`. Un skill nommé `self` reste adressable par son ID canonique.

## Protection des fichiers et reprise

- Une autre source avec le même nom, un dossier tiers ou un fichier tiers bloque la copie, même avec `--force` ou un contenu identique.
- Une modification locale suivie bloque install/update/remove. `--force` autorise son remplacement ou retrait avec une sauvegarde dont le chemin est affiché.
- Les fichiers étrangers ajoutés dans un dossier géré sont conservés. La suppression retire uniquement les fichiers suivis et les répertoires devenus vides.
- Le dry-run résout les sources et vérifie le plan sans aucune écriture persistante. Une transaction interrompue bloque le dry-run jusqu’à restauration par une vraie mutation.
- Une invocation d’installation est une transaction pour tous ses skills et destinations. Un échec restaure les écritures et l’état. `update` applique une transaction indépendante par installation et signale les succès et les installations conservées en cas d’échec partiel.
- La prochaine mutation restaure un journal interrompu avant le réseau. Si un fichier ou l’état a changé depuis l’interruption, STH bloque la restauration, conserve le journal et indique le chemin divergent. Conserver ces sauvegardes et comparer les versions avant de relancer ; ne pas supprimer aveuglément le journal.

Les fichiers STH résident sous `global/`, à côté de `settings.json` : `state.json`, `lock`, `transactions/`, `backups/` et `provenance/`. L’override existant `STH_USER_CONFIG_DIR=<base>` donne `<base>/sth/global`. Les racines autorisées sont recalculées à partir du home courant ; un chemin enregistré n’est jamais une autorisation de mutation. Un déplacement de home est signalé. Aucun fichier du projet courant, y compris `.sth/last-operation.json`, n’est écrit.

Les changements entre plusieurs volumes ne sont pas instantanément visibles : un agent ouvert peut voir une étape intermédiaire. Le contrat garantit la cohérence après succès ou restauration. Les mutations STH sont verrouillées et les plans périmés refusés.

## Contenu et limites V1

Tous les fichiers d’une installation proviennent d’un même SHA complet. Les octets et bits exécutables utiles sont préservés sans en-tête STH. Les licences/notices à la racine du dépôt sont conservées à part dans `provenance/`. Le mode Git reste enregistré ; sous Windows, la comparaison des permissions tient compte du seul attribut de lecture seule exposé par le système.

Ponytail sélectionne `skills/ponytail`. `--path skills --skill '*'` sélectionne les six dossiers canoniques, sans copies OpenClaw ni `skills/plugin.json`. STH ne lance aucun script téléchargé. Les hooks, plugins, règles toujours actives, paramètres de confiance et MCP restent hors périmètre.

Le `SKILL.md` à la racine d’un dépôt n’est pas accepté. Les liens symboliques, sous-modules, arbres tronqués, traversées, collisions de casse/normalisation Unicode et chemins non portables sont refusés explicitement. Les fichiers `plugin.json`, `gemini-extension.json` et `hooks.json` à l’intérieur d’un bundle sont refusés conservativement, même s’ils servent d’exemples ; les déclarations de hooks dans le frontmatter sont également refusées. Les champs d’extension sont préservés et signalés.

Limites : 16 Mio par réponse HTTP, 64 Mio cumulés et 4096 fichiers par bundle, provenance incluse. GitLab, Azure DevOps, Bitbucket, STH Cloud global, retrait sélectif par agent et activation permanente sont hors V1.

## JSON et codes de sortie

`--json` écrit une seule enveloppe sur stdout, même en erreur. Les éventuels messages auxiliaires vont sur stderr. Champs stables : `schema_version: 1`, `scope: "global"`, `operation`, `outcome`, `resources`, `warnings`, `errors`. Chaque ressource expose identité, source, références, SHA, consommateurs, placements, destinations, état des fichiers, niveau de validation et éventuel chemin de sauvegarde. `status` ajoute les constats par fichier et l’état distant.

Codes : `0` succès, `1` erreur opérationnelle/conflit/échec partiel, `2` syntaxe ou sélection invalide. Aide : `sth help install`. Les scripts `sth completion bash`, `zsh` et `powershell` contiennent les options globales.
