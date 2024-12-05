package git

import (
	"context"
	"fmt"
	"github.com/google/go-github/v56/github"
	"net/http"
)

// Issue represents the generic issue across Git platforms like GitHub, GitLab, etc.
type Issue struct {
	Number      int
	Title       string // Issue title
	Description string // Issue description
	State       string // Issue state (e.g., "open", "closed")
	HasPR       bool   // Whether the issue has an associated PR or merge request
	URL         string // URL of the issue on the platform
}

// The IssueClient interface defines an interface for issuers in Git, such as GitHub or GitLab.
type IssueClient interface {
	// List retrieves a list of issues from the specified GitHub repository.
	// Parameters:
	// - ctx: The context for the request.
	// - owner: The owner of the repository.
	// - repo: The name of the repository.
	// Returns:
	// - A slice of pointers to GitHub issues.
	// - An error if the operation fails.
	List(ctx context.Context, owner, repo string) ([]*Issue, error)

	// Create creates a new issue in the specified GitHub repository.
	// Parameters:
	// - ctx: The context for the request.
	// - owner: The owner of the repository.
	// - repo: The name of the repository.
	// - title: The title of the new issue.
	// - body: The body content of the new issue.
	// Returns:
	// - A pointer to the created GitHub issue.
	// - An error if the operation fails.
	Create(ctx context.Context, owner, repo, title, body string) (*Issue, error)

	// Edit modifies the body of an existing issue in the specified GitHub repository.
	// Parameters:
	// - ctx: The context for the request.
	// - owner: The owner of the repository.
	// - repo: The name of the repository.
	// - issueNumber: The number of the issue to edit.
	// - body: The new content for the issue body.
	// Returns:
	// - A pointer to the updated GitHub issue.
	// - An error if the operation fails.
	Edit(ctx context.Context, owner, repo string, issueNumber int, body string) (*Issue, error)

	// Close closes an existing issue in the specified GitHub repository.
	// Parameters:
	// - ctx: The context for the request.
	// - owner: The owner of the repository.
	// - repo: The name of the repository.
	// - issueNumber: The number of the issue to close.
	// Returns:
	// - A pointer to the closed GitHub issue.
	// - An error if the operation fails.
	Close(ctx context.Context, owner, repo string, issueNumber int) (*Issue, error)
}

// GitHubIssueClient defines a specific IssueClient implementation for GitHub.
type GitHubIssueClient struct {
	Client *github.Client
}

func mapGitHubIssue(ghIssue *github.Issue) *Issue {
	if ghIssue == nil {
		return nil
	}
	return &Issue{
		Number:      ghIssue.GetNumber(),
		Title:       ghIssue.GetTitle(),
		Description: ghIssue.GetBody(),
		State:       ghIssue.GetState(),
		HasPR:       ghIssue.GetPullRequestLinks() != nil,
		URL:         ghIssue.GetHTMLURL(),
	}
}

func (c *GitHubIssueClient) List(ctx context.Context, owner, repo string) ([]*Issue, error) {
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

	var platformIssues []*Issue
	for _, ghIssue := range issues {
		platformIssues = append(platformIssues, mapGitHubIssue(ghIssue))
	}

	return platformIssues, nil
}

// Create creates a new issue in a GitHub repository
func (c *GitHubIssueClient) Create(ctx context.Context, owner, repo, title, body string) (*Issue, error) {
	issueRequest := &github.IssueRequest{Title: &title, Body: &body}
	ghIssue, response, err := c.Client.Issues.Create(ctx, owner, repo, issueRequest)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to create issue: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to create issue: %v", err)
	}

	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create issue: unexpected status code %d", response.StatusCode)
	}

	return mapGitHubIssue(ghIssue), nil
}

func (c *GitHubIssueClient) Edit(ctx context.Context, owner, repo string, issueNumber int, body string) (*Issue, error) {
	// Prepare the edit request
	editRequest := &github.IssueRequest{Body: &body}

	// Call the GitHub API to edit the issue
	ghIssue, response, err := c.Client.Issues.Edit(ctx, owner, repo, issueNumber, editRequest)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to edit issue: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to edit issue: %v", err)
	}

	// Check if the response status is successful
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to edit issue: unexpected status code %d", response.StatusCode)
	}

	// Convert the GitHub issue to the generic Issue type
	return mapGitHubIssue(ghIssue), nil
}

func (c *GitHubIssueClient) Close(ctx context.Context, owner, repo string, issueNumber int) (*Issue, error) {
	// Prepare the request to close the issue
	state := "closed"
	closeRequest := &github.IssueRequest{State: &state}

	// Call the GitHub API to close the issue
	ghIssue, response, err := c.Client.Issues.Edit(ctx, owner, repo, issueNumber, closeRequest)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("failed to close issue: %s, %v", response.Status, err)
		}
		return nil, fmt.Errorf("failed to close issue: %v", err)
	}

	// Check if the response status is successful
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to close issue: unexpected status code %d", response.StatusCode)
	}

	// Convert the GitHub issue to the generic Issue type
	return mapGitHubIssue(ghIssue), nil
}
