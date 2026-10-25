# Global CLI: trace RED/GREEN

Exécution le 25 octobre 2026. Les tests ci-dessous ont été écrits et exécutés avant les comportements correspondants. Les tests utilisent des homes/configurations/projets temporaires et des serveurs HTTP locaux ; aucun `t.Parallel` n’est ajouté.

| Incrément | RED constaté | GREEN |
| --- | --- | --- |
| Options typées, valeurs répétables, syntaxe et codes | `go test ./cmd -run '^TestGlobalOptions' -count=1` : APIs `parseGlobalOptions`/`ExitCode` absentes | Même commande passée après parsing minimal |
| Portée avant projet, JSON erreurs/succès sans effets de bord | `go test ./cmd -run '^TestGlobalRouting' -count=1` : ancien JSON local `[]`, enveloppe globale absente | Même commande passée après routage et inventaire initial |
| Install multi-destinations et dry-run | `go test ./cmd -run '^TestGlobalInstallDryRunAndAllAgents$' -count=1` : installation encore indisponible dans le routage incrémental | Même test passé avec les APIs source/FS intégrées ; binaire, exécutable et provenance vérifiés |
| List/status/update/remove, idempotence, ajout de consommateur, pin et force | `go test ./cmd -run '^TestGlobal(InstallDryRun|Lifecycle|Pin)' -count=1` : install passait, status/update étaient encore absents | `go test ./cmd -run '^TestGlobal(Install|Lifecycle|Pin|Routing|Options|AcceptanceResult)' -count=1` passé |
| Sélection guidée, `--yes`/JSON sans sélection implicite, aide et complétion | `go test ./cmd -run '^TestGlobal(Interactive|YesAndJSON|HelpAndCompletion)' -count=1` : sélection interactive refusée, aide sans `--global` | Même commande passée avec deux choix successifs agents/skills |
| Réinstallation de plusieurs SHA, disparition en amont, wildcard ancien + nouveau | `go test ./cmd -run TestGlobalReinstall -count=1` : skill ancien introuvable, wildcard perdait les anciens consommateurs | `go test ./cmd -run '^TestGlobal' -count=1` passé après résolution par identité existante et découverte des nouvelles sources |
| Restauration avant tentative réseau | `go test ./cmd -run '^TestGlobalAcceptance(AddAgentToSeveral|RecoversInterrupted|RecoveryPreserves|UpdateRepairs)' -count=1` : état interrompu conservé lors d’une panne réseau | Même suite passée après restauration préalable des vraies mutations |
| Détail des conflits en texte | `go test ./cmd -run '^TestGlobalTextReportsConflictPath$' -count=1` : seule l’erreur agrégée était visible | Même test passé avec cause et chemin affichés |

Les tests d’acceptation indépendants et leurs observations sont décrits dans [global-review.md](global-review.md). Les contrats source et FS ont leurs propres cycles dans [global-source-tdd.md](global-source-tdd.md), [global-fs-tdd.md](global-fs-tdd.md) et [global-registry-tdd.md](global-registry-tdd.md).

Deux types d’échecs intermédiaires ne sont pas revendiqués comme RED fonctionnel : les compilations pendant que les APIs d’un autre chantier étaient en cours, et le rejet macOS de `/var` dans le chemin temporaire de configuration. Ce dernier a été corrigé par un test de régression FS dédié. La première suite complète a également trouvé une régression d’aide locale (`sth install <resource-ref>` attendu) introduite pendant l’intégration ; le libellé local a été conservé et le test repassé.

Les checks de régression ajoutés qui passaient déjà (par exemple retrait de fichiers disparus en amont) sont documentés comme vérifications, sans inventer un cycle RED/GREEN.
