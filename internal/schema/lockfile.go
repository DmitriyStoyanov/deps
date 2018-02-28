package schema

import (
	"fmt"
	"sort"
	"strings"
)

type Lockfile struct {
	Current *LockfileVersion `json:"current"`
	Updated *LockfileVersion `json:"updated,omitempty"`
}

type LockfileVersion struct {
	Fingerprint  string                        `json:"fingerprint"`
	Dependencies map[string]LockfileDependency `json:"dependencies"`
}

// Dependency stores data for a manifest or lockfile dependency (some fields will be empty)
type LockfileDependency struct {
	*Dependency
	Installed    Version `json:"installed"`
	IsTransitive bool    `json:"is_transitive,omitempty"`
	Constraint   string  `json:"constraint,omitempty"`
}

// GetDependencyTypeString returns a string representation of the dependencies relationship to the repo
func (dep *LockfileDependency) GetDependencyTypeString() string {
	if dep.IsTransitive {
		return "transitive"
	}

	return "direct"
}

// LockfileChanges stores data about what changes were made to a lockfile
type LockfileChanges struct {
	Updated []string
	Added   []string
	Removed []string
}

func (lockfile *Lockfile) changesByType() map[string]*LockfileChanges {
	changesByType := map[string]*LockfileChanges{}

	for name, dep := range lockfile.Current.Dependencies {
		depType := dep.GetDependencyTypeString()

		_, ok := changesByType[depType]
		if !ok {
			changesByType[depType] = &LockfileChanges{}
		}
		changesForType := changesByType[depType]

		if updatedDep, found := lockfile.Updated.Dependencies[name]; !found {
			changesForType.Removed = append(changesForType.Removed, name)
		} else {
			if dep.Installed.Name != updatedDep.Installed.Name {
				changesForType.Updated = append(changesForType.Updated, name)
			}
		}
	}

	for name, dep := range lockfile.Updated.Dependencies {
		if _, found := lockfile.Current.Dependencies[name]; !found {
			depType := dep.GetDependencyTypeString()

			_, ok := changesByType[depType]
			if !ok {
				changesByType[depType] = &LockfileChanges{}
			}
			changesForType := changesByType[depType]

			changesForType.Added = append(changesForType.Added, name)
		}
	}

	return changesByType
}

// GetSummaryLine returns a summary line for a bulleted markdown list
func (lockfile *Lockfile) GetSummaryLine(lockfilePath string) (string, error) {
	changesByType := lockfile.changesByType()
	additional := ""
	if direct, found := changesByType["direct"]; found && len(direct.Updated) > 0 {
		additional = fmt.Sprintf(" (including %d updated direct dependencies)", len(direct.Updated))
	}
	return fmt.Sprintf("- `%v` was updated%v", lockfilePath, additional), nil
}

// GetBodyContent compiles the long-form content for changes to the lockfile
func (lockfile *Lockfile) GetBodyContent(lockfilePath string) (string, error) {
	changesByType := lockfile.changesByType()

	contentParts := []string{}

	contentParts = append(contentParts, "#### "+lockfilePath)

	if transitive, found := changesByType["transitive"]; found {
		contentParts = append(contentParts, fmt.Sprintf("%d transitive dependencies were updated, %d were added, and %d removed. View the git diff for more details about exactly what changed.", len(transitive.Updated), len(transitive.Added), len(transitive.Removed)))
	}

	if direct, found := changesByType["direct"]; found && len(direct.Updated) > 0 {
		contentParts = append(contentParts, fmt.Sprintf("The following %d direct dependencies were updated:", len(direct.Updated)))

		sort.Strings(direct.Updated) // sort first to get predictable order
		for _, name := range direct.Updated {
			dep := lockfile.Updated.Dependencies[name]
			versions := []Version{dep.Installed}
			versionContent := dep.GetMarkdownContentForVersions(name, versions)
			contentParts = append(contentParts, fmt.Sprintf("**%s**\n\n%s", name, versionContent))
		}
	}

	return strings.Join(contentParts, "\n\n"), nil
}