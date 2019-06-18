package runner

import (
	"github.com/dropseed/deps/internal/config"
	"github.com/dropseed/deps/internal/output"
	"github.com/dropseed/deps/internal/schema"
)

var lockfileUpdatesDisabled = false
var manifestUpdatesDisabled = false

type Updates []*Update

func (updates Updates) printOverview() {
	if len(updates) < 1 {
		output.Success("No updates found")
	}

	for _, update := range updates {
		id := update.dependencies.GetID()
		title, err := update.dependencies.GenerateTitle()
		if err != nil {
			panic(err)
		}
		output.Event("[%s] %s", id, title)
	}
}

func newUpdatesFromDependencies(dependencies *schema.Dependencies, dependencyConfig *config.Dependency) (Updates, error) {
	updates := Updates{}

	if *dependencyConfig.LockfileUpdates.Enabled && !lockfileUpdatesDisabled {
		for path, lockfile := range dependencies.Lockfiles {
			if lockfile.Updated == nil || len(lockfile.Updated.Dependencies) < 1 {
				continue
			}

			updateDependencies := schema.Dependencies{
				Lockfiles: map[string]*schema.Lockfile{
					path: lockfile,
				},
			}

			update := Update{
				dependencies:     &updateDependencies,
				dependencyConfig: dependencyConfig,
			}

			updates = append(updates, &update)
		}
	}

	if *dependencyConfig.ManifestUpdates.Enabled && !manifestUpdatesDisabled {
		for path, manifest := range dependencies.Manifests {

			if manifest.Updated == nil || len(manifest.Updated.Dependencies) < 1 {
				continue
			}

			filteredGroups, err := dependencyConfig.ManifestUpdates.FilteredDependencyGroups(manifest.Updated.Dependencies)
			if err != nil {
				return nil, err
			}

			for _, groupDeps := range filteredGroups {

				updateDependencies := schema.Dependencies{
					Manifests: map[string]*schema.Manifest{
						path: &schema.Manifest{
							LockfilePath: manifest.LockfilePath,
							Current: &schema.ManifestVersion{
								Dependencies: map[string]*schema.ManifestDependency{},
							},
							Updated: &schema.ManifestVersion{
								Dependencies: map[string]*schema.ManifestDependency{},
							},
						},
					},
				}

				for name, dep := range groupDeps {
					updateDependencies.Manifests[path].Current.Dependencies[name] = manifest.Current.Dependencies[name]
					updateDependencies.Manifests[path].Updated.Dependencies[name] = dep
				}

				update := Update{
					dependencies:     &updateDependencies,
					dependencyConfig: dependencyConfig,
				}

				updates = append(updates, &update)
			}
		}
	}

	return updates, nil
}
