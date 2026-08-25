# skills-transfer-hub (`sth`)

CLI Go de transfert de skills/agents/commands. Backend Supabase Edge Functions (`sth-back`), dashboard Next.js (`sth-dashboard`).

## Style Go (strict)

Deux règles dures pour tout code Go écrit ou modifié ici. Détails et exemples dans le skill `go-style` (`.agents/skills/go-style/SKILL.md`).

### Pas de variable à une lettre

Noms descriptifs partout (`file`, `buffer`, `line`, `record`, `command`…). Seules exceptions autorisées :

- receivers : `func (a *app)`, `func (c *Client)`
- `t *testing.T` / `b *testing.B`
- `err`
- index de boucle `for i := 0; ...`. Une boucle `for _, x := range` prend un nom descriptif, pas une lettre.

### Pas de commentaire dans le code

Zéro commentaire, doc-comments godoc des symboles exportés inclus. Le code se documente par des noms explicites ; si un commentaire semble nécessaire, extraire/renommer jusqu'à ce que le code soit lisible seul.

> `.github/instructions/go.instructions.md` reste plus permissif ; en cas de conflit, ces deux règles priment.

## Build / test

- `go build ./...`
- `go test ./internal/fs/... ./cmd/...`
- Les tests `cmd` utilisent massivement `t.Setenv` (sandbox `STH_USER_CONFIG_DIR`), donc séquentiels — pas de `t.Parallel()`.
