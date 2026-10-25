package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type GlobalConsumer struct {
	ID              string   `json:"id"`
	Family          string   `json:"family"`
	ReadRoots       []string `json:"read_roots"`
	WriteRoots      []string `json:"write_roots"`
	Supported       bool     `json:"supported"`
	ValidationLevel string   `json:"validation_level"`
	Reason          string   `json:"reason,omitempty"`
}

type GlobalRoot struct {
	ID           string `json:"id"`
	RelativePath string `json:"relative_path"`
}

type GlobalDestination struct {
	RootID             string   `json:"root_id"`
	Root               string   `json:"root"`
	RelativePath       string   `json:"relative_path"`
	RequestedConsumers []string `json:"requested_consumers"`
	PossibleReaders    []string `json:"possible_readers"`
}

func GlobalConsumers() []GlobalConsumer {
	return []GlobalConsumer{
		{ID: "claude", Family: "claude", ReadRoots: []string{"claude"}, WriteRoots: []string{"claude"}, Supported: true, ValidationLevel: "documented"},
		{ID: "codex", Family: "codex", ReadRoots: []string{"agents"}, WriteRoots: []string{"agents"}, Supported: true, ValidationLevel: "documented"},
		{ID: "cursor", Family: "cursor", ReadRoots: []string{"cursor", "agents", "claude", "codex-legacy"}, WriteRoots: []string{"agents"}, Supported: true, ValidationLevel: "documented"},
		{ID: "gemini", Family: "gemini", ReadRoots: []string{"agents", "gemini"}, WriteRoots: []string{"agents"}, Supported: true, ValidationLevel: "documented"},
		{ID: "copilot-cli", Family: "copilot", ReadRoots: []string{"copilot", "agents"}, WriteRoots: []string{"agents"}, Supported: true, ValidationLevel: "documented"},
		{ID: "copilot-vscode", Family: "copilot", ReadRoots: []string{"copilot", "agents", "claude"}, WriteRoots: []string{"agents"}, Supported: true, ValidationLevel: "documented"},
		{ID: "antigravity-ide", Family: "antigravity", ReadRoots: []string{"antigravity-ide", "antigravity-legacy"}, WriteRoots: []string{"antigravity-ide"}, Supported: true, ValidationLevel: "documented"},
		{ID: "antigravity-cli", Family: "antigravity", ReadRoots: []string{"antigravity-cli"}, WriteRoots: []string{"antigravity-cli"}, Supported: true, ValidationLevel: "documented"},
	}
}

func GlobalRoots(home string) map[string]string {
	return map[string]string{
		"agents":          filepath.Join(home, ".agents", "skills"),
		"claude":          filepath.Join(home, ".claude", "skills"),
		"antigravity-ide": filepath.Join(home, ".gemini", "config", "skills"),
		"antigravity-cli": filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
	}
}

func ResolveGlobalConsumers(ids []string, all bool) ([]string, []string, error) {
	if all && len(ids) != 0 {
		return nil, nil, fmt.Errorf("--all-agents cannot be combined with --agent")
	}
	if !all && len(ids) == 0 {
		return nil, nil, fmt.Errorf("global installation requires --agent or --all-agents")
	}
	selected := map[string]bool{}
	warnings := []string{}
	for _, consumer := range GlobalConsumers() {
		if all {
			if consumer.Supported {
				selected[consumer.ID] = true
			} else {
				warnings = append(warnings, fmt.Sprintf("excluded %s: %s", consumer.ID, consumer.Reason))
			}
		}
	}
	for _, requested := range ids {
		matched := false
		for _, consumer := range GlobalConsumers() {
			if requested != consumer.ID && requested != consumer.Family {
				continue
			}
			matched = true
			if !consumer.Supported {
				return nil, nil, fmt.Errorf("global consumer %s is unsupported: %s", consumer.ID, consumer.Reason)
			}
			selected[consumer.ID] = true
		}
		if !matched {
			return nil, nil, fmt.Errorf("unknown global consumer %q", requested)
		}
	}
	consumers := make([]string, 0, len(selected))
	for consumer := range selected {
		consumers = append(consumers, consumer)
	}
	sort.Strings(consumers)
	if len(consumers) == 0 {
		return nil, nil, fmt.Errorf("no supported global consumers selected")
	}
	warnings = append(warnings, "Global skill locations are documented; discovery-unverified: loading and invocation have not been verified in agent runtimes. Open a new agent session or reload skills.")
	return consumers, warnings, nil
}

func PlanGlobalDestinations(home, skillName string, consumers []string) ([]GlobalDestination, []string, error) {
	if !filepath.IsAbs(home) {
		return nil, nil, fmt.Errorf("global home must be absolute: %q", home)
	}
	if err := validateAgentSkillName("global", skillName); err != nil {
		return nil, nil, err
	}
	if isGlobalReservedName(skillName) {
		return nil, nil, fmt.Errorf("skill name %q is reserved on Windows", skillName)
	}
	selected, warnings, err := ResolveGlobalConsumers(consumers, false)
	if err != nil {
		return nil, nil, err
	}
	roots := GlobalRoots(home)
	byRoot := map[string]*GlobalDestination{}
	for _, requested := range selected {
		for _, consumer := range GlobalConsumers() {
			if consumer.ID != requested {
				continue
			}
			for _, rootID := range consumer.WriteRoots {
				destination := byRoot[rootID]
				if destination == nil {
					destination = &GlobalDestination{RootID: rootID, Root: roots[rootID], RelativePath: skillName}
					byRoot[rootID] = destination
				}
				destination.RequestedConsumers = append(destination.RequestedConsumers, requested)
			}
		}
	}
	destinations := make([]GlobalDestination, 0, len(byRoot))
	for rootID, destination := range byRoot {
		for _, consumer := range GlobalConsumers() {
			for _, readRootID := range consumer.ReadRoots {
				if readRootID == rootID {
					destination.PossibleReaders = append(destination.PossibleReaders, consumer.ID)
				}
			}
		}
		sort.Strings(destination.PossibleReaders)
		destinations = append(destinations, *destination)
	}
	sort.Slice(destinations, func(first, second int) bool { return destinations[first].RootID < destinations[second].RootID })
	warnings = append(warnings, globalCoexistenceWarnings(home, skillName, destinations)...)
	return destinations, warnings, nil
}

func isGlobalReservedName(name string) bool {
	switch strings.ToLower(name) {
	case "con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return true
	}
	return false
}

func globalReadRoots(home string) map[string]string {
	roots := GlobalRoots(home)
	for rootID, relativePath := range map[string]string{
		"cursor": ".cursor/skills", "copilot": ".copilot/skills", "gemini": ".gemini/skills",
		"codex-legacy": ".codex/skills", "antigravity-legacy": ".gemini/antigravity/skills",
	} {
		roots[rootID] = filepath.Join(home, filepath.FromSlash(relativePath))
	}
	return roots
}

func globalCoexistenceWarnings(home, skillName string, destinations []GlobalDestination) []string {
	warnings := []string{}
	plannedRoots := map[string]bool{}
	readers := map[string]bool{}
	for _, destination := range destinations {
		plannedRoots[destination.RootID] = true
		for _, reader := range destination.PossibleReaders {
			readers[reader] = true
		}
	}
	roots := globalReadRoots(home)
	for _, consumer := range GlobalConsumers() {
		if !readers[consumer.ID] {
			continue
		}
		plannedCount := 0
		for _, rootID := range consumer.ReadRoots {
			if plannedRoots[rootID] {
				plannedCount++
				continue
			}
			skillPath := filepath.Join(roots[rootID], skillName)
			exists, err := inspectGlobalReadPath(home, skillPath)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s coexistence inspection unavailable at %s: %v", consumer.ID, skillPath, err))
				continue
			}
			if exists {
				message := fmt.Sprintf("%s coexistence: another skill exists at %s; preserved and not adopted", consumer.ID, skillPath)
				if consumer.ID == "gemini" && plannedRoots["agents"] && rootID == "gemini" {
					message += "; documented .agents alias takes precedence over .gemini at user scope"
				}
				warnings = append(warnings, message)
			}
		}
		if plannedCount > 1 {
			warnings = append(warnings, fmt.Sprintf("%s may discover duplicate entries from %d planned roots; host deduplication and invocation are unverified", consumer.ID, plannedCount))
		}
	}
	sort.Strings(warnings)
	return warnings
}

func inspectGlobalReadPath(home, candidate string) (bool, error) {
	relativePath, err := filepath.Rel(home, candidate)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false, fmt.Errorf("read path is outside home")
	}
	currentPath := home
	for _, component := range strings.Split(relativePath, string(filepath.Separator)) {
		currentPath = filepath.Join(currentPath, component)
		info, err := os.Lstat(currentPath)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			return false, fmt.Errorf("symbolic or irregular path %s was not followed", currentPath)
		}
	}
	return true, nil
}
