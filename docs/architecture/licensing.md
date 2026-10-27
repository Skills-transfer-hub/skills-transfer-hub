 # Architecture — Licences, plans & authentification

> État des lieux factuel de l'auth, des licences et des plans à travers les trois composants STH : le CLI Go (`skills-transfer-hub`), le backend Deno/Supabase (`sth-back`) et le dashboard Next.js (`sth-dashboard`). Référence pour toute décision future de gating, filtrage ou évolution du modèle d'accès.
>
> **Date de rédaction** : 2026-05-11. Toute divergence observée ultérieurement doit être traitée comme une dérive à corriger ici **ou** dans le code.

## Sommaire

1. [Vue d'ensemble](#1-vue-densemble)
2. [Format et cycle de vie des clés](#2-format-et-cycle-de-vie-des-clés)
3. [Plans et quotas](#3-plans-et-quotas)
4. [Endpoints CLI ↔ back](#4-endpoints-cli--back)
5. [Comportement du CLI face au plan](#5-comportement-du-cli-face-au-plan)
6. [Rôles côté dashboard](#6-rôles-côté-dashboard)
7. [Anomalies et points d'attention](#7-anomalies-et-points-dattention)
8. [Invariants à préserver](#8-invariants-à-préserver)
9. [Décisions ouvertes](#9-décisions-ouvertes)

---

## 1. Vue d'ensemble

```
                                       ┌────────────────────────────────┐
                                       │  sth-dashboard (Next.js 16)    │
                                       │                                │
                              ┌───────►│  /dashboard/licenses           │
                              │        │   └─ createLicense() ──────────┼──┐
                              │        │                                │  │
                  Stripe ─────┤        │  /dashboard/billing            │  │
                  (Checkout,  │        │   └─ Customer Portal           │  │
                  Portal,     │        │                                │  │
                  Webhooks)   │        │  /api/webhooks/stripe          │  │
                              │        └────────────────────────────────┘  │
                              │                                            │
                              │                                            │  INSERT
                              ▼                                            ▼
                  ┌──────────────────────────────────────────────────────────┐
                  │  Supabase (Postgres + Storage + RLS)                     │
                  │                                                          │
                  │  organizations · licenses · subscriptions · invoices     │
                  │  skills · skill_versions · skill_installs                │
                  │  cli_usage_records · stripe_events                       │
                  └──────────────────────────────────────────────────────────┘
                              ▲                                            ▲
                              │ service-role                               │ service-role
                              │                                            │
                  ┌────────────────────────────────┐                       │
                  │  sth-back (Deno Edge Functions)│                       │
                  │                                │                       │
                  │  /v1/access-check              │                       │
                  │  /v1/catalog                   │                       │
                  │  /v1/skills/<id>   (quota)     │                       │
                  │  /v1/license/validate          │                       │
                  │  /v1/usage                     │───────────────────────┘
                  └────────────────────────────────┘
                              ▲
                              │ HTTPS  +  Authorization: Bearer lsk_live_…
                              │
                  ┌────────────────────────────────┐
                  │  sth CLI (Go)                  │
                  │                                │
                  │  sth login  → keychain OS      │
                  │  sth … commands                │
                  └────────────────────────────────┘
```

Le dashboard et le back parlent tous deux à Supabase mais **jamais directement entre eux** : la frontière de cohérence est la base. Le CLI ne connaît que le back.

---

## 2. Format et cycle de vie des clés

### 2.1. Format

- **Préfixe** : `lsk_live_` (9 caractères, constant)
- **Corps** : 24 octets aléatoires (`crypto.randomBytes`) encodés en base32 RFC 4648 sans padding
- **Alphabet base32** : `ABCDEFGHIJKLMNOPQRSTUVWXYZ234567` — choix délibéré pour éviter les confusions visuelles `0/O`, `1/I/l`, et permettre la copie au terminal
- **Display prefix** : 16 caractères (`lsk_live_` + 7 chars base32 = ~35 bits d'entropie pour l'affichage, faible collision intra-org)

Génération côté dashboard : `sth-dashboard/src/lib/services/license-key.ts:15-27`.

### 2.2. Stockage

| Emplacement | Donnée | Forme |
|---|---|---|
| `licenses.key_hash` | SHA-256 hex du token entier | Irréversible |
| `licenses.key_prefix` | 16 premiers caractères du plaintext | Display uniquement |
| Réponse HTTP de création | `plaintextKey` | **Une seule fois** (jamais re-fetchable) |
| Keychain OS du poste utilisateur | Plaintext intégral | Via `zalando/go-keyring` |

Insert : `sth-dashboard/src/lib/services/license.service.ts:46-75` (`createLicense`).

### 2.3. Authentification d'une requête CLI

Lookup côté back (`sth-back/shared/auth.ts:28-92`) :

1. Extraction du header `Authorization: Bearer <token>` — `extractBearer` rejette tout token qui ne commence pas par `lsk_live_` (`shared/auth.ts:22-23`)
2. Hash SHA-256 du token reçu
3. `SELECT … FROM licenses WHERE key_hash = ?` (`shared/auth.ts:33-37`)
4. Check `expiration_date < today` → `401 expired` (`shared/auth.ts:45-48`)
5. Fire-and-forget `UPDATE licenses SET last_used_at = NOW()` (`shared/auth.ts:51-56`)
6. Fetch `organizations` puis `subscriptions` pour calculer le plan effectif

### 2.4. Validation côté CLI

Le CLI vérifie le préfixe en local avant d'appeler le back : `skills-transfer-hub/cmd/login.go:48` (`!strings.HasPrefix(key, "lsk_live_")` → `clé invalide`).

### 2.5. Révocation

**Aucun flag `revoked_at` ou `disabled`** dans le schéma. Pour invalider une clé :

- Supprimer la row dans `licenses`
- Ou antidater `expiration_date` (next `/v1/access-check` renverra `401 expired`)

Conséquence : pas de trace d'audit "clé révoquée le X par Y" — voir [§9](#9-décisions-ouvertes).

---

## 3. Plans et quotas

### 3.1. Énum unique

Trois tiers, **strictement identiques** entre back et dashboard :

```ts
type PlanTier = 'starter' | 'pro' | 'team'
```

- Back : `sth-back/shared/types.ts` (importé par `quotas.ts` et `auth.ts`)
- Dashboard : `sth-dashboard/src/lib/billing/plans.ts:7`

### 3.2. Quotas serveur

Source unique côté back (`sth-back/shared/quotas.ts:3-7`) :

| Plan | `maxSkills` |
|---|---|
| `starter` | 3 |
| `pro` | 10 |
| `team` | `Infinity` |

### 3.3. Plan effectif

Calculé à chaque appel authentifié (`sth-back/shared/auth.ts:76-79`) :

```ts
const plan: PlanTier = sub?.status === "active"
  ? (sub.plan_tier as PlanTier)
  : (org.plan_tier as PlanTier);
```

Une org sans subscription Stripe active (cas `starter` gratuit ou `past_due`) retombe sur `organizations.plan_tier`. Pas de grace period.

### 3.4. Facturation

La disponibilité commerciale des plans et l'intégration du prestataire de paiement vivent dans le dashboard, hors de ce dépôt. Le CLI ne connaît que le plan effectif renvoyé par le back.

---

## 4. Endpoints CLI ↔ back

| Endpoint | Auth | Gate plan | Comportement |
|---|---|---|---|
| `POST /v1/access-check` | ✅ | ❌ (retourné mais non utilisé pour gater) | Renvoie `{ok, org, plan, expires_at}` (`functions/cli/routes/access_check.ts:4-11`) |
| `GET /v1/catalog` | ✅ | ❌ | Filtrage par visibilité uniquement (`visibility='public' OR owner_org_id=ctx.org.id`). Voir [§7](#7-anomalies-et-points-dattention) — tout le catalog est exposé peu importe le tier |
| `GET /v1/skills/<id>` | ✅ | ✅ | **Seul endpoint actuellement gated par plan** : `402 quota_exceeded` si `installsCount >= quotaFor(plan).maxSkills` (`functions/cli/routes/catalog_get.ts:48-56`). Réinstall idempotente (`onConflict: "license_id,skill_id"`). Renvoie une URL Storage signée 5 min |
| `POST /v1/license/validate` | ✅ | ❌ | Diagnostic : renvoie `{plan, quota, installs_count, expires_at, subscription_status}` |
| `POST /v1/usage` | ✅ | ❌ | Best-effort, max 100 records par batch. Insert dans `cli_usage_records (member_id, organization_id, command_timestamp, cli_version)` |

### 4.1. Erreurs

Codes définis dans `sth-back/shared/errors.ts:1-49` :

| Code | HTTP | Levé par |
|---|---|---|
| `missing_authorization` | 401 | `extractBearer` (pas de header) |
| `invalid_authorization` | 401 | `extractBearer` (schéma ou préfixe KO) |
| `unknown_key` | 401 | `resolveAuth` (hash non trouvé) |
| `expired` | 401 | `resolveAuth` (date passée) |
| `quota_exceeded` | 402 | `catalog_get` |
| `not_found` | 404 | `catalog_get` (skill privé non-membre, ou inexistant) |
| `bad_request` | 400 | Payload mal formé |
| `internal` | 500 | Erreur DB |

Les codes `no_subscription` et `method_not_allowed` sont définis mais jamais levés.

---

## 5. Comportement du CLI face au plan

### 5.1. Ce que le CLI reçoit

`internal/http/cloud.go:29-34` :

```go
type AccessCheckResult struct {
    OK        bool           `json:"ok"`
    Org       AccessCheckOrg `json:"org"`
    Plan      string         `json:"plan"`
    ExpiresAt string         `json:"expires_at"`
}
```

### 5.2. Ce que le CLI en fait

**Un seul site d'usage de `Plan`** dans tout le code Go (vérifié par `grep`) :

```go
// cmd/login.go:71-74
a.writeStdout(
    fmt.Sprintf("Connecté à %s (plan %s, expire le %s).", resp.Org.Name, resp.Plan, resp.ExpiresAt),
    "Clé stockée dans le keychain de l'OS.",
)
```

- Affiché une fois au login
- **Non persisté** — `AccessCheckResult` est jeté après l'écriture stdout
- **Aucun helper `requirePlan(...)`** — aucun gating client de quelque feature que ce soit

### 5.3. Conséquence pratique

Toute restriction d'usage est **server-authoritative**. Le CLI n'a aucune connaissance locale du plan entre deux logins. Implication directe pour l'UX :

- `sth pull <skill>` peut planter avec `402 quota_exceeded` sans signal préalable
- Aucun moyen d'afficher "Cette feature est réservée au plan Pro" avant l'appel
- L'utilisateur ne sait pas combien de skills il lui reste avant d'atteindre `maxSkills`

### 5.4. À ne pas confondre — `alpha_gate.go`

`skills-transfer-hub/cmd/alpha_gate.go` refuse de tourner si le binaire est en retard sur la dernière release GitHub. C'est une **restriction de version**, pas de plan. Bypass via `STH_ALPHA_GATE=0`. À ignorer dans toute réflexion sur le licensing.

---

## 6. Rôles côté dashboard

Trois rôles définis : `owner | admin | member`.

| Page | Owner | Admin | Member |
|---|---|---|---|
| `/dashboard/licenses` | Toutes les clés de l'org + génération + assignation | Idem owner | Sa propre clé uniquement, lecture seule |
| `/dashboard/billing` | Vue complète + Customer Portal + cancel | Lecture seule | Lecture seule |
| `/dashboard/members` | Invite/role change/remove | Invite/role change/remove | Lecture |
| `/dashboard/seats` | Toggle "consume pending invites" | Idem owner | Lecture |
| `/dashboard/usage` | Tous les membres (30j) | Tous les membres | Sa propre activité uniquement |

Authentification : Clerk + JWT template → Supabase RLS. Pages protégées par middleware Clerk dans `sth-dashboard/src/middleware.ts`.

---

## 7. Anomalies et points d'attention

### 7.1. Drift de schéma — résolu (migrations 0007-0011)

Le code de `sth-back/shared/auth.ts` et `license.service.ts` utilise les colonnes `licenses.key_hash`, `licenses.key_prefix`, `licenses.last_used_at`. Ces colonnes sont créées par `0007_licenses_hash_columns.sql`, et `0011_drop_license_plaintext.sql` retire l'ancien `key_value`. Un setup neuf reconstruit donc le schéma d'auth correct depuis `sth-dashboard/supabase/migrations/` sans récupération manuelle.

### 7.2. Tables du catalog — présentes dans le schéma versionné

`sth-back/functions/cli/routes/catalog_get.ts` et `catalog_list.ts` queryent :

- `skills` (avec colonnes `id, slug, resource_type, folder_name, name, description, latest_version, visibility, owner_org_id`)
- `skill_versions` (`storage_path, sha256, version`)
- `skill_installs` (`license_id, skill_id, version` avec UNIQUE sur `(license_id, skill_id)`)

Ces tables sont créées par `0008_skills_catalog.sql` (skills + skill_versions) et `0010_skill_installs_table.sql` (skill_installs, avec l'index `skill_installs_license_idx`).

### 7.3. Catalog non filtré par plan

`/v1/catalog` renvoie tous les skills `public` + ceux de l'org du caller. Un utilisateur `starter` voit donc le même catalog qu'un `team`, mais se prendra un `402` à l'install une fois le quota dépassé. C'est une **incohérence UX** plus qu'un bug — voir [§9](#9-décisions-ouvertes).

### 7.4. Expiration dure

`license.expiration_date < today` → `401` immédiat (`auth.ts:45-48`). Pas de grace period, pas de warning préventif côté serveur. Le dashboard calcule `expiring` (≤30j) côté client (`license.service.ts:13-18`) mais cette information n'est **pas** transmise au CLI.

### 7.5. Pas de scopes par clé

Une clé = accès complet à toutes les ressources de l'org. Pas de notion de "clé read-only", "clé CI", "clé personnelle vs partagée". Le champ `assignee_id` est purement informatif (qui détient cette clé) — il n'altère pas les permissions.

### 7.6. Pas de plaintext-recovery

Une fois la modale "Copie maintenant" fermée, **plus aucun moyen** de récupérer la clé. C'est un choix de sécurité explicite (`license.service.ts:42-44`). Le user doit régénérer une clé.

---

## 8. Invariants à préserver

Ces propriétés sont vraies aujourd'hui et leur rupture casserait la cohérence du système. À vérifier avant tout PR qui touche au licensing.

1. **`PlanTier` strictement identique** entre `sth-back/shared/types.ts` et `sth-dashboard/src/lib/billing/plans.ts`. Ajouter un tier nécessite un changement coordonné dans les deux repos **et** une mise à jour de `PLAN_QUOTAS`.
2. **Le plaintext de la clé n'est jamais persisté** en DB ni en log. Le seul `key` plaintext existant après création est dans le keychain de l'utilisateur.
3. **`key_hash` est la seule clé de lookup** dans `licenses`. Aucun code ne doit faire `WHERE key = …` ou `WHERE key_prefix = …` pour l'auth.
4. **Le plan effectif** est calculé en lisant `subscriptions.plan_tier` (si `status='active'`) puis fallback `organizations.plan_tier`. Ne jamais lire le plan depuis le JWT Clerk ou un cache côté front.
5. **`/v1/access-check`** est le seul endpoint qui ne fait pas de side-effect modifiant (à part `last_used_at` touch). Garder cette propriété pour permettre des health checks répétés.
6. **Le CLI ne stocke pas le plan**. Si on décide de le persister un jour, le faire dans `~/.sth/` (pas dans le keychain — le keychain ne sert qu'aux secrets) et avec une TTL courte.

---

## 9. Décisions ouvertes

Liste des chantiers explicitement non décidés, à arbitrer indépendamment.

| Sujet | Question | Impact |
|---|---|---|
| Persistance du plan côté CLI | Stocker `plan` + `expires_at` dans `~/.sth/state.json` après login ? | Permettrait `sth status`, gating client, warnings d'expiration locaux |
| Filtrage catalog par plan | `/v1/catalog` doit-il masquer les skills inaccessibles au tier courant ? | UX plus cohérente, mais double la logique de gating (catalog + install) |
| `requirePlan(...)` côté CLI | Bloquer certaines commandes locales selon le plan ? | Nécessite §1 ci-dessus. Question UX : duplication avec serveur ou complément ? |
| Révocation propre | Ajouter `licenses.revoked_at` + filtrer dans `resolveAuth` ? | Audit trail, possibilité de "désactivation temporaire" |
| Grace period | Tolérer N jours après expiration avec warning ? | Réduit la friction commerciale mais ouvre une fenêtre d'abus |
| Scopes par clé | `licenses.scopes TEXT[]` (`read`, `usage:report`, `admin`) ? | Permet clés CI restreintes. Gros chantier — pas justifié tant qu'il n'y a qu'un type d'usage |
| Ouverture commerciale Pro/Team | Quand basculer `comingSoon: false` ? Avec quel changement de quota ? | Hors scope tech, mais conditionne les autres décisions |

---

## Références fichiers

### CLI (`skills-transfer-hub`)

- `cmd/login.go:48` — validation préfixe `lsk_live_`
- `cmd/login.go:71-74` — affichage unique du plan
- `cmd/alpha_gate.go` — gating de version (ne concerne pas le licensing)
- `internal/http/cloud.go:29-34` — type `AccessCheckResult`
- `internal/http/cloud.go:46-73` — appel `/v1/access-check`
- `internal/auth/keyring.go` — wrappers `zalando/go-keyring`

### Back (`sth-back`)

- `functions/cli/index.ts:18-71` — routeur, dispatcher, `resolveAuth` en entrée
- `shared/auth.ts:15-26` — `extractBearer`
- `shared/auth.ts:28-92` — `resolveAuth`
- `shared/quotas.ts:3-11` — `PLAN_QUOTAS`, `quotaFor`
- `functions/cli/routes/access_check.ts:4-11` — payload de réponse
- `functions/cli/routes/catalog_get.ts:34-56` — quota check (`402`)
- `shared/errors.ts:1-49` — codes d'erreur
