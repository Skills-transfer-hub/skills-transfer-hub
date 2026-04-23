# Research: Guided Repository Onboarding

## Decision 1: Faire de `loreskills` un point d'entrée guidé dépendant du TTY

- **Decision**: `loreskills` sans argument ouvre un menu guidé state-aware quand `stdin` et `stdout` sont interactifs; hors TTY, la commande imprime une guidance courte indiquant l'état courant et la prochaine action utile.
- **Rationale**: Cela respecte la clarification validée, améliore l'onboarding sans casser les usages scriptés, et reste simple à implémenter en stdlib via `bufio.Reader` et un routeur d'actions dans `cmd/`.
- **Alternatives considered**: Conserver la sortie minimale historique a été rejeté car cela laisse l'utilisateur sans guidance. Ajouter une commande séparée `guide` a été rejeté car l'utilisateur a explicitement choisi `loreskills` comme point d'entrée. Rendre le menu obligatoire hors TTY a été rejeté car cela casserait les pipelines et les tests non interactifs.

## Decision 2: Centraliser la guidance provider dans un catalogue structuré aligné sur les docs officielles

- **Decision**: Définir un catalogue provider structuré pour les quatre hosts supportés (`github.com`, `gitlab.com`, `dev.azure.com`, `bitbucket.org`) contenant le label, le host, l'URL de documentation officielle, l'URL ou le chemin d'action à ouvrir, la permission minimale de lecture, le format attendu du `repository_id`, et les variables d'environnement recommandées. Pour Bitbucket Cloud, migrer la guidance d'App Password vers les API tokens avec permission `Repositories: Read`.
- **Rationale**: La guidance est aujourd'hui dispersée dans `cmd/init.go` et Bitbucket pointe encore vers un modèle ancien. Un catalogue structuré réduit la dérive, permet de réutiliser les mêmes données dans `init`, le menu racine, `help` et les tests, et ancre les futures mises à jour sur des pages officielles vérifiables.
- **Alternatives considered**: Laisser des chaînes inline dans `init.go` a été rejeté car cela complique la maintenance et la validation. Déplacer toute la guidance dans le README uniquement a été rejeté car le besoin porte sur une guidance in-product. Ouvrir maintenant le support self-hosted a été rejeté car l'utilisateur a limité la portée aux hosts hébergés.

## Decision 3: Garder `SKILLS.md` prioritaire, avec fallback de scan provider-aware si le catalogue est absent

- **Decision**: Si `SKILLS.md` existe et se parse correctement, LoreSkills s'appuie exclusivement sur ce catalogue. Si `SKILLS.md` est absent, LoreSkills bascule sur un scan provider-aware qui énumère les chemins correspondant à `category/skill-folder/SKILL.md`. Si `SKILLS.md` existe mais contient des entrées invalides, LoreSkills retourne l'erreur sans fusion silencieuse avec le scan.
- **Rationale**: Cette stratégie respecte la clarification `SKILLS.md` first, préserve la compatibilité avec les dépôts déjà catalogués, et ajoute une voie de compatibilité réaliste pour les dépôts structurés sans manifeste explicite.
- **Alternatives considered**: `SKILLS.md` obligatoire a été rejeté car le besoin impose une compatibilité plus souple. Le scan seul a été rejeté car `SKILLS.md` doit rester la source explicite quand il est présent. Fusionner catalogue et scan a été rejeté car cela créerait des ambiguïtés difficiles à expliquer et à tester.

## Decision 4: Synchroniser des bundles complets ancrés par `SKILL.md`, pas seulement un fichier markdown

- **Decision**: Normaliser chaque ressource découverte en bundle racine `category/skill-folder/`, avec `SKILL.md` comme point d'entrée requis. `install` et `update` synchronisent tous les fichiers de ce dossier racine, pas seulement `SKILL.md`, et stockent un manifeste précis des fichiers gérés pour permettre l'écrasement et le nettoyage ciblé.
- **Rationale**: Le besoin de protéger le code/plugin géré n'est pas satisfaisable avec le modèle actuel single-file. Le sync par bundle permet de suivre les fichiers compagnons, d'écraser proprement les fichiers gérés, et de supprimer les fichiers obsolètes qui disparaissent du dépôt source.
- **Alternatives considered**: Continuer à installer seulement `SKILL.md` a été rejeté car cela ne couvre ni le code compagnon ni la protection demandée. Copier récursivement sans manifeste a été rejeté car cela empêcherait de distinguer fichiers gérés et non gérés lors des updates.

## Decision 5: Appliquer une politique de frontière gérée stricte avec écrasement des contenus gérés

- **Decision**: Toute écriture gérée doit se résoudre sous `<target_dir>`. Les fichiers suivis dans le manifeste de bundle sont considérés comme propriété du dépôt source et sont écrasés lors d'un `install` ou `update`; toute tentative d'écriture hors frontière gérée est bloquée; les fichiers obsolètes encore présents dans le manifeste sont supprimés pendant la resynchronisation du bundle.
- **Rationale**: Cela aligne l'implémentation sur la clarification explicite de l'utilisateur: les modifications locales dans la zone gérée ne bloquent pas l'opération. La frontière de chemin reste le vrai mécanisme de protection.
- **Alternatives considered**: Demander une confirmation à chaque conflit a été rejeté par clarification. Tolérer des chemins hors frontière a été rejeté pour raison de sécurité. Ne jamais supprimer les fichiers obsolètes a été rejeté car cela laisserait dériver le contenu géré.

## Decision 6: Cacher l'état de mise à jour stable par machine via `os.UserCacheDir`

- **Decision**: Ajouter un cache machine-local `update-check.json` sous `os.UserCacheDir()/loreskills/` contenant `last_checked_at`, la dernière version stable connue, l'état (`no-update`, `update-available`, `error`) et les URLs d'asset/checksum nécessaires à l'offre d'update. Le menu racine le consulte; si l'état est frais (<24 h), aucun appel réseau n'est refait au démarrage interactif.
- **Rationale**: Le check doit être automatique mais borné dans le temps. Un cache machine-local évite d'alourdir le profil projet avec une donnée purement locale et garde un démarrage réactif.
- **Alternatives considered**: Checker à chaque lancement a été rejeté car trop coûteux et bavard. Ne checker que sur action explicite a été rejeté car cela ne correspond plus au comportement automatique demandé. Stocker cet état dans `.loreskills/project.json` a été rejeté car la donnée est machine-locale, pas projet.

## Decision 7: Étendre `internal/http/` avec les APIs de listing des providers hébergés pour le scan et la synchro de bundles

- **Decision**: Ajouter des endpoints provider-aware de listing d'arborescence dans `internal/http/` pour les quatre hosts supportés, en réutilisant le modèle d'authentification déjà en place. Les chemins dérivés servent à détecter les bundles compatibles, à lister les fichiers d'un bundle, puis à télécharger chaque fichier utile pendant `install` et `update`.
- **Rationale**: Les URLs raw actuelles permettent de lire un fichier, pas d'énumérer un dépôt. Le fallback scan et la synchro de bundle exigent une vue arborescente du dépôt source.
- **Alternatives considered**: Sheller vers `git clone` a été rejeté car cela élargit la surface d'exécution, alourdit les tests et complique Windows. Exiger un clone local préalable a été rejeté car l'outil doit rester autonome. Abandonner le fallback scan a été rejeté car ce point fait partie du besoin clarifié.
