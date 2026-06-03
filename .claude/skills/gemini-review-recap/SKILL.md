---
name: gemini-review-recap
description: Format du commentaire récapitulatif à poster sur une PR après avoir traité une review Gemini (ou un autre bot de review) sur sth-back / skills-transfer-hub / sth-dashboard. À utiliser à chaque fois qu'on poste un récap de fixes suite à une review.
---

# Récap de fixes après review Gemini

Quand on pousse des fixes pour une review Gemini sur l'un des trois repos STH (`sth-back`, `skills-transfer-hub`, `sth-dashboard`) et qu'on relance `/gemini review`, le commentaire de PV **doit** suivre cette structure.

```markdown
Récapitulatif des fixes — N passes Gemini + M fixes CI

<Phrase d'intro qui résume le périmètre : combien de retours traités, quels commits, avant la nouvelle review.>

Round 1 — commit `<sha>`

| # | Sévérité | Fichier | Fix livré |
|---|---|---|---|
| 1 | high | `path/to/file.go` | Description concrète du changement : nommer le helper/fonction/migration introduit, le fichier où il vit, et la raison en une phrase courte. Pas de « corrigé » / « addressed » — décrire ce qui change. |
| 2 | medium | `…` | … |

Round 2 — commit `<sha>`

| # | Sévérité | Fichier | Fix livré |
| … |

/gemini review
```

## Conventions

- Titre : `Récapitulatif des fixes — N passes Gemini + M fixes CI` (compter les échecs CI séparément des rounds Gemini ; omettre `+ M fixes CI` s'il n'y en a pas).
- Une phrase d'intro résumant le périmètre avant toute table.
- **Une table par commit**, avec le SHA du commit dans le titre du round.
- Colonnes toujours dans cet ordre : `#`, `Sévérité`, `Fichier`, `Fix livré`.
- `Sévérité` reprend les labels Gemini **verbatim** (`high`, `medium`, `security-medium`, `security-high`…).
- `Fichier` : chemin avec nom de fichier (+ sous-dossier si utile), en backticks.
- `Fix livré` : le changement concret — nommer le helper / la fonction extraite / la migration, raison en une phrase ; pas de verbes abstraits (« fixé », « traité »).
- Table simple, pas de puces imbriquées dans une cellule.
- Terminer par `/gemini review` seul sur sa ligne pour relancer la review.

Objectif : propre et scannable — chaque fix vérifiable d'un coup d'œil.
