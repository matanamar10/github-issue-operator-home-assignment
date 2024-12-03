package controller

import (
	"context"
	"github.com/google/go-github/v56/github"
)

// The IssueClient interface define interface for issuer in git: either github or gitlab.
type IssueClient interface {
	ListIssues(ctx context.Context, owner, repo string) ([]*github.Issue, error)
	CreateIssue(ctx context.Context, owner, repo string, title, body string) (*github.Issue, error)
	EditIssue(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.Issue, error)
	CloseIssue(ctx context.Context, owner, repo string, issueNumber int) (*github.Issue, error)
}

// GitHubIssueClient Define specific IssueClient which implemented IssueClient interface
type GitHubIssueClient struct {
	Client *github.Client
}

func (c *GitHubIssueClient) ListIssues(ctx context.Context, owner, repo string) ([]*github.Issue, error) {
	issues, _, err := c.Client.Issues.ListByRepo(ctx, owner, repo, nil)
	return issues, err
}

func (c *GitHubIssueClient) CreateIssue(ctx context.Context, owner, repo string, title, body string) (*github.Issue, error) {
	issueRequest := &github.IssueRequest{Title: &title, Body: &body}
	issue, _, err := c.Client.Issues.Create(ctx, owner, repo, issueRequest)
	return issue, err
}

func (c *GitHubIssueClient) EditIssue(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.Issue, error) {
	editRequest := &github.IssueRequest{Body: &body}
	issue, _, err := c.Client.Issues.Edit(ctx, owner, repo, issueNumber, editRequest)
	return issue, err
}

func (c *GitHubIssueClient) CloseIssue(ctx context.Context, owner, repo string, issueNumber int) (*github.Issue, error) {
	state := "closed"
	closeRequest := &github.IssueRequest{State: &state}
	issue, _, err := c.Client.Issues.Edit(ctx, owner, repo, issueNumber, closeRequest)
	return issue, err
}
