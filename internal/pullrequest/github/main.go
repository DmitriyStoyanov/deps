package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/dependencies-io/deps/internal/env"
	"github.com/dependencies-io/deps/internal/pullrequest"
)

// PullRequest stores additional GitHub specific data
type PullRequest struct {
	// directly use the properties of base Pullrequest
	*pullrequest.Pullrequest
	RepoOwnerName string
	RepoName      string
	RepoFullName  string
	APIToken      string
	Number        int
	CreatedAt     string
}

// NewPullrequestFromDependenciesJSONPathAndEnv creates a PullRequest
func NewPullrequestFromDependenciesJSONPathAndEnv(dependenciesJSONPath string) (*PullRequest, error) {
	prBase, err := pullrequest.NewPullrequestFromJSONPathAndEnv(dependenciesJSONPath)
	if err != nil {
		return nil, err
	}

	fullName := os.Getenv("GITHUB_REPO_FULL_NAME")
	parts := strings.Split(fullName, "/")
	return &PullRequest{
		Pullrequest:   prBase,
		RepoOwnerName: parts[0],
		RepoName:      parts[1],
		RepoFullName:  fullName,
		APIToken:      os.Getenv("GITHUB_API_TOKEN"),
	}, nil
}

func (pr *PullRequest) getCreateJSONData() []byte {
	var base string
	if base = env.GetSetting("GITHUB_BASE_BRANCH", ""); base == "" {
		base = pr.DefaultBaseBranch
	}

	pullrequestMap := map[string]string{
		"title": pr.Title,
		"head":  pr.Branch,
		"base":  base,
		"body":  pr.Body,
	}
	fmt.Printf("%+v\n", pullrequestMap)
	pullrequestData, _ := json.Marshal(pullrequestMap)
	return pullrequestData
}

func (pr *PullRequest) addHeadersToRequest(req *http.Request) {
	req.Header.Add("Authorization", "token "+pr.APIToken)
	req.Header.Add("User-Agent", "dependencies.io pullrequest")
	req.Header.Set("Content-Type", "application/json")
}

func (pr *PullRequest) createPR() (map[string]interface{}, error) {
	// open the actual PR, first of two API calls

	pullrequestData := pr.getCreateJSONData()
	pullrequestsURL := fmt.Sprintf("https://api.github.com/repos/%v/pulls", pr.RepoFullName)

	client := &http.Client{}

	req, err := http.NewRequest("POST", pullrequestsURL, bytes.NewBuffer(pullrequestData))
	if err != nil {
		return nil, err
	}

	pr.addHeadersToRequest(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("failed to create pull request: %+v", resp)
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return data, nil
}

// Create performs the creation of the PR on GitHub
func (pr *PullRequest) Create() error {
	// check the optional settings now, before actually creating the PR (which we'll have to update)
	var labels []string
	if labelsEnv := env.GetSetting("GITHUB_LABELS", ""); labelsEnv != "" {
		if err := json.Unmarshal([]byte(labelsEnv), &labels); err != nil {
			return err
		}
	}

	var assignees []string
	if assigneesEnv := env.GetSetting("GITHUB_ASSIGNEES", ""); assigneesEnv != "" {
		if err := json.Unmarshal([]byte(assigneesEnv), &assignees); err != nil {
			return err
		}
	}

	var milestone int64
	if milestoneEnv := env.GetSetting("GITHUB_MILESTONE", ""); milestoneEnv != "" {
		var err error
		milestone, err = strconv.ParseInt(milestoneEnv, 10, 32)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Preparing to open GitHub pull request for %v\n", pr.RepoFullName)

	if !env.IsProduction() {
		fmt.Printf("Skipping GitHub API call due to \"%v\" env\n", env.GetCurrentEnv())
		pr.Action.Name = "PR #0"
		return nil
	}

	data, err := pr.createPR()
	if err != nil {
		return err
	}

	pr.Number = int(data["number"].(float64))
	pr.CreatedAt = data["created_at"].(string)

	// set the Action info for reporting back to dependencies.io
	pr.Action.Name = fmt.Sprintf("PR #%v", pr.Number)
	pr.Action.Metadata["github_pull_request"] = data

	// pr has been created at this point, now have to add meta fields in
	// another request
	issueURL, _ := data["issue_url"].(string)
	htmlURL, _ := data["html_url"].(string)
	fmt.Printf("Successfully created %v\n", htmlURL)

	if len(labels) > 0 || len(assignees) > 0 || milestone > 0 {
		issueMap := make(map[string]interface{})

		if len(labels) > 0 {
			issueMap["labels"] = labels
		}
		if len(assignees) > 0 {
			issueMap["assignees"] = assignees
		}
		if milestone > 0 {
			issueMap["milestone"] = milestone
		}

		fmt.Printf("%+v\n", issueMap)
		issueData, _ := json.Marshal(issueMap)

		req, err := http.NewRequest("PATCH", issueURL, bytes.NewBuffer(issueData))
		if err != nil {
			return err
		}

		pr.addHeadersToRequest(req)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != 200 {
			return fmt.Errorf("failed to update pull request: %+v", resp)
		}

		fmt.Printf("Successfully updated PR fields on %v\n", htmlURL)
	}

	return nil
}

// DoRelated performs the related PR behavior set by the user
func (pr *PullRequest) DoRelated() error {
	// related pr behavior is valid
	relatedPRBehavior := env.GetSetting("related_pr_behavior", "close")
	if relatedPRBehavior == "" {
		return nil
	}

	if relatedPRBehavior != "close" {
		return errors.New("\"close\" is the only supported GitHub related PR behavior")
	}

	issue, err := pr.getRelatedPR()
	if err != nil {
		return err
	}
	if issue == nil {
		fmt.Printf("No related PR found.\n")
		return nil
	}

	if relatedPRBehavior == "close" {
		err := pr.closePR(issue.GetNumber())
		if err != nil {
			return err
		}

		comment := fmt.Sprintf("This PR has been automatically closed in favor of #%v.", pr.Number)
		err = pr.commentOnIssue(issue.GetNumber(), comment)
		if err != nil {
			return err
		}

	}

	return nil
}