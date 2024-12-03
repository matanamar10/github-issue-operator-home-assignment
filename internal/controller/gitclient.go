package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v56/github"
)

// GitHubIssueClient defines the specific IssueClient which implements the IssueClient interface.
type GitHubIssueClient struct {
	Client *github.Client
}

// ListIssues lists all issues in the repository.
func (c *GitHubIssueClient) ListIssues(ctx context.Context, owner, repo string) ([]*github.Issue, error) {
	issues, response, err := c.Client.Issues.ListByRepo(ctx, owner, repo, nil)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to list issues: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to list issues: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list issues: unexpected status code %d", response.StatusCode)
	}

	return issues, nil
}

// CreateIssue creates a new issue in the repository.
func (c *GitHubIssueClient) CreateIssue(ctx context.Context, owner, repo, title, body string) (*github.Issue, error) {
	issueRequest := &github.IssueRequest{
		Title: &title,
		Body:  &body,
	}

	issue, response, err := c.Client.Issues.Create(ctx, owner, repo, issueRequest)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to create issue: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to create issue: %v", err)
	}

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create issue: unexpected status code %d", response.StatusCode)
	}

	return issue, nil
}

// EditIssue edits an existing issue in the repository.
func (c *GitHubIssueClient) EditIssue(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.Issue, error) {
	editRequest := &github.IssueRequest{
		Body: &body,
	}

	issue, response, err := c.Client.Issues.Edit(ctx, owner, repo, issueNumber, editRequest)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to edit issue: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to edit issue: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to edit issue: unexpected status code %d", response.StatusCode)
	}

	return issue, nil
}

// CloseIssue closes an issue in the repository.
func (c *GitHubIssueClient) CloseIssue(ctx context.Context, owner, repo string, issueNumber int) (*github.Issue, error) {
	state := "closed"
	closeRequest := &github.IssueRequest{
		State: &state,
	}

	issue, response, err := c.Client.Issues.Edit(ctx, owner, repo, issueNumber, closeRequest)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to close issue: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to close issue: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to close issue: unexpected status code %d", response.StatusCode)
	}

	return issue, nil
}
