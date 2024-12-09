package controller

import (
	"context"
	"fmt"
	"github.com/matanamar10/github-issue-operator-hhome-assignment/internal/git"
	"go.uber.org/zap"
	"strings"
)

// parseRepoURL parses a repository URL and extracts the owner and repository name.
// Returns an error if the URL format is invalid.
func parseRepoURL(repoURL string) (string, string, error) {
	parts := strings.Split(repoURL, "/")
	if len(parts) < 5 {
		return "", "", fmt.Errorf("invalid repository URL: %s", repoURL)
	}
	return parts[3], parts[4], nil
}

// fetchIssuesFromGit fetches issues from Git and updates the allIssues slice
func (r *GithubIssueReconciler) fetchIssuesFromGit(ctx context.Context, owner, repo string) ([]*git.Issue, error) {
	fetchedIssues, fetchErr := r.IssueClient.List(ctx, owner, repo)
	if fetchErr != nil {
		r.Log.Warn("Failed to fetch issues, retrying", zap.Error(fetchErr))
		return nil, fetchErr
	}

	return fetchedIssues, nil
}
