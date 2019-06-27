package runner

import (
	"errors"
	"fmt"
	"os"

	"github.com/dropseed/deps/internal/component"
	"github.com/dropseed/deps/internal/config"
	"github.com/dropseed/deps/internal/output"
)

const COLLECTOR = "collector"
const ACTOR = "actor"

func getConfig() (*config.Config, error) {
	cfg, err := config.NewConfigFromPath(config.DefaultFilename, nil)
	if os.IsNotExist(err) {
		output.Event("No local config found, detecting your dependencies automatically")
		// should we always check for inferred? and could let them know what they
		// don't have in theirs?
		// dump both to yaml, use regular diff tool and highlighting?
		cfg, err = config.InferredConfigFromDir(".")
		if err != nil {
			return nil, err
		}

		inferred, err := cfg.DumpYAML()
		if err != nil {
			return nil, err
		}
		println("---")
		println(inferred)
		println("---")
	} else if err != nil {
		return nil, err
	}

	if len(cfg.Dependencies) < 1 {
		return nil, errors.New("no dependencies found")
	}

	cfg.Compile()

	return cfg, nil
}

func collectUpdates() (Updates, Updates, error) {
	cfg, err := getConfig()
	if err != nil {
		return nil, nil, err
	}

	availableUpdates, err := getAvailableUpdates(cfg)
	if err != nil {
		return nil, nil, err
	}

	newUpdates := Updates{}      // PRs for these
	existingUpdates := Updates{} // lockfile update on these?

	for _, update := range availableUpdates {
		if update.branchExists() {
			existingUpdates.addUpdate(update)
		} else {
			newUpdates.addUpdate(update)
		}
	}

	if len(existingUpdates) > 0 {
		fmt.Println()
		output.Event("%d existing updates", len(existingUpdates))
		existingUpdates.printOverview()
	}

	if len(newUpdates) > 0 {
		fmt.Println()
		output.Event("%d new updates to be made", len(newUpdates))
		newUpdates.printOverview()
	}

	return newUpdates, existingUpdates, nil
}

func getAvailableUpdates(cfg *config.Config) (Updates, error) {
	availableUpdates := Updates{}

	for index, dependencyConfig := range cfg.Dependencies {

		runner, err := component.NewRunnerFromString(dependencyConfig.Type)
		if err != nil {
			return nil, err
		}
		env, err := dependencyConfig.Environ()
		if err != nil {
			return nil, err
		}
		runner.Index = index
		runner.Env = env

		// add a .shouldInstall - true when local or ref changed?

		if err := runner.Install(); err != nil {
			return nil, err
		}

		dependencies, err := runner.Collect(dependencyConfig.Path)
		if err != nil {
			return nil, err
		}

		updates, err := newUpdatesFromDependencies(dependencies, dependencyConfig)
		if err != nil {
			return nil, err
		}

		if len(updates) > 0 {
			for _, update := range updates {
				// Store this for use later
				update.runner = runner
				availableUpdates.addUpdate(update)
			}
		}
	}

	return availableUpdates, nil
}
