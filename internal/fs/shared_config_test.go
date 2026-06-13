package fs

import "testing"

func hasMergeEntry(entries []ConfigMergeEntry, kind ConfigEntryKind, field string, action MergeAction) bool {
	for _, entry := range entries {
		if entry.Kind == kind && entry.Field == field && entry.Action == action {
			return true
		}
	}
	return false
}

func TestMergeOrgConfig_OrgOverwritesLocalProvider(t *testing.T) {
	local := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "acme", Provider: "github", CatalogRef: "main"}},
	}
	org := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "acme", Provider: "github", CatalogRef: "release/1.x"}},
	}

	result := MergeOrgConfig(local, org)

	merged, found := providerByID(result.Merged.Providers, "acme")
	if !found {
		t.Fatal("le provider acme doit subsister après le merge")
	}
	if merged.CatalogRef != "release/1.x" {
		t.Fatalf("l'org doit gagner : CatalogRef = %q, attendu release/1.x", merged.CatalogRef)
	}
	if !hasMergeEntry(result.Entries, ConfigEntryProvider, "acme", MergeActionOverwrite) {
		t.Fatalf("entrée overwrite attendue pour acme, entries = %+v", result.Entries)
	}
}

func TestMergeOrgConfig_KeepsLocalOnlyResources(t *testing.T) {
	local := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "local-fork", Provider: "github"}},
		Resources:     []ResourceEntry{{ResourceName: "scratch", ProviderID: "local-fork"}},
	}
	org := ProjectConfigV2{SchemaVersion: 2}

	result := MergeOrgConfig(local, org)

	if _, found := providerByID(result.Merged.Providers, "local-fork"); !found {
		t.Fatal("un provider purement local doit être préservé")
	}
	if _, found := resourceByName(result.Merged.Resources, "scratch"); !found {
		t.Fatal("une ressource purement locale doit être préservée")
	}
	if !hasMergeEntry(result.Entries, ConfigEntryProvider, "local-fork", MergeActionKeep) {
		t.Fatalf("entrée keep attendue pour local-fork, entries = %+v", result.Entries)
	}
}

func TestMergeOrgConfig_OrgPinWins(t *testing.T) {
	local := ProjectConfigV2{
		SchemaVersion: 2,
		Resources:     []ResourceEntry{{ResourceName: "reviewer", ProviderID: "acme", PinnedVersion: "v1.0.0"}},
	}
	org := ProjectConfigV2{
		SchemaVersion: 2,
		Resources:     []ResourceEntry{{ResourceName: "reviewer", ProviderID: "acme", PinnedVersion: "v1.4.0"}},
	}

	result := MergeOrgConfig(local, org)

	merged, found := resourceByName(result.Merged.Resources, "reviewer")
	if !found {
		t.Fatal("la ressource reviewer doit subsister")
	}
	if merged.PinnedVersion != "v1.4.0" {
		t.Fatalf("le pin org doit gagner : PinnedVersion = %q, attendu v1.4.0", merged.PinnedVersion)
	}
}

func TestShareableSubset_DropsScannerHintAndDeepCopies(t *testing.T) {
	local := ProjectConfigV2{
		SchemaVersion: 2,
		Providers:     []ProviderEntry{{ID: "acme", Provider: "github", CatalogRef: "main"}},
		Resources:     []ResourceEntry{{ResourceName: "reviewer", ProviderID: "acme", PinnedVersion: "v1"}},
		ScannerHint:   "local-only-hint",
	}

	subset := ShareableSubset(local)

	if subset.ScannerHint != "" {
		t.Fatalf("ScannerHint ne doit pas être partagé, got %q", subset.ScannerHint)
	}
	if len(subset.Providers) != 1 || len(subset.Resources) != 1 {
		t.Fatalf("le sous-ensemble doit porter providers et resources, got %+v", subset)
	}
	subset.Providers[0].CatalogRef = "mutated"
	if local.Providers[0].CatalogRef != "main" {
		t.Fatal("ShareableSubset doit copier les providers indépendamment du local")
	}
}
