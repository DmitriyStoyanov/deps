package github

import "testing"

func TestNoopDereference(t *testing.T) {
	body := "hey this is normal\n\nwith newlines"
	cleaned, err := dereferenceGitHubIssueLinks(body)
	if err != nil {
		t.Error(err)
	}
	if body != cleaned {
		t.Fail()
	}
}

func TestDereference(t *testing.T) {
	body := "hey this is normal\n\n[with](https://github.com/test-org/repo/issues/45) newlines"
	cleaned, err := dereferenceGitHubIssueLinks(body)
	if err != nil {
		t.Error(err)
	}
	if cleaned != "hey this is normal\n\n[with](https://www.dependencies.io/github-redirect/test-org/repo/issues/45) newlines" {
		t.Error(cleaned)
	}
}