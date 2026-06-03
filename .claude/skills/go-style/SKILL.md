---
name: go-style
description: Conventions Go strictes du repo skills-transfer-hub (CLI `sth`). À utiliser pour écrire ou refactoriser du code Go dans ce repo. Deux règles dures — aucune variable à une lettre (sauf idiomes Go listés) et aucun commentaire dans le code.
---

# Style Go — skills-transfer-hub

Deux règles non négociables pour tout code Go écrit ou modifié dans ce repo.

## 1. Pas de variable à une lettre

Tout identifiant local porte un nom descriptif : `file`, `buffer`, `line`, `record`, `command`, `index`…

Exceptions idiomatiques **autorisées** (et uniquement celles-ci) :

- **Receivers** : `func (a *app)`, `func (c *Client)` — cohérence avec le reste du codebase.
- **`t *testing.T`** (et `b *testing.B`) dans les tests.
- **`err`** pour les erreurs.
- **Index de boucle** : `for i := 0; i < n; i++`. Une boucle `for _, x := range` prend un nom descriptif (`for _, record := range records`), pas une lettre.

Exemples :

```go
// au lieu de
f, err := os.OpenFile(path, ...)
for _, l := range lines { ... }
for _, c := range commands { ... }

// écrire
file, err := os.OpenFile(path, ...)
for _, line := range lines { ... }
for _, command := range commands { ... }
```

## 2. Pas de commentaire dans le code

Zéro commentaire, **y compris les doc-comments godoc** sur les symboles exportés. Le code se documente par des noms de fonctions, de types et de variables explicites.

```go
// au lieu de
// ReadUsageLog returns the most-recent entries, newest-first.
func ReadUsageLog(limit int) ([]UsageLogRecord, error) { ... }

// écrire
func ReadUsageLog(limit int) ([]UsageLogRecord, error) { ... }
```

Si une logique semble nécessiter un commentaire, extraire une fonction nommée ou renommer les variables jusqu'à ce que le code soit lisible seul.

## Portée

- Aucun linter n'impose ces règles dans le repo (rien ne casse au build/CI), mais elles sont obligatoires en revue.
- `.github/instructions/go.instructions.md` reste plus permissif (variables courtes en boucle, commentaires « si nécessaire ») ; **ce skill prime** pour le nouveau code.
