package controller

import (
	"context"
	"fmt"
	"github.com/google/go-github/v56/github"
	issues "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"time"
)

// searchForIssue checks if GithubIssue CRD has an issue in the repo
func searchForIssue(issue *issues.GithubIssue, gitHubIssues []*github.Issue) *github.Issue {
	for _, ghIssue := range gitHubIssues {
		if ghIssue != nil && strings.EqualFold(*ghIssue.Title, issue.Spec.Title) {
			return ghIssue
		}
	}
	return nil
}

// UpdateIssueStatus updates the status of the GithubIssue CRD
func (r *GithubIssueReconciler) UpdateIssueStatus(ctx context.Context, issue *issues.GithubIssue, githubIssue *github.Issue) error {
	PRChange := r.CheckForPR(githubIssue, issue)
	OpenChange := r.CheckIfOpen(githubIssue, issue)

	if PRChange || OpenChange {
		r.Log.Info("Updating Issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))

		if err := r.Client.Status().Update(ctx, issue); err != nil {
			r.Log.Warn("Status update failed, attempting fallback", zap.Error(err))

			if fallbackErr := r.Client.Update(ctx, issue); fallbackErr != nil {
				r.Recorder.Event(issue, corev1.EventTypeWarning, "StatusUpdateFailed", fmt.Sprintf("Failed to update status: %v", fallbackErr))
				return fmt.Errorf("failed to update status: %v", fallbackErr)
			}
		}

		r.Recorder.Event(issue, corev1.EventTypeNormal, "StatusUpdated", "Issue status updated successfully")
		r.Log.Info("Issue status updated successfully", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
	} else {
		r.Log.Info("No changes detected in issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
	}

	return nil
}

// CheckIfOpen checks if the issue is open
func (r *GithubIssueReconciler) CheckIfOpen(githubIssue *github.Issue, issueObject *issues.GithubIssue) bool {
	if githubIssue == nil {
		return false
	}
	state := githubIssue.GetState()
	condition := &metav1.Condition{
		Type:    "IssueIsOpen",
		Status:  metav1.ConditionTrue,
		Reason:  "IssueIsOpen",
		Message: "Issue is open",
	}
	if state != "open" {
		condition = &metav1.Condition{
			Type:    "IssueIsOpen",
			Status:  metav1.ConditionFalse,
			Reason:  fmt.Sprintf("IssueIs%s", state),
			Message: fmt.Sprintf("Issue is %s", state),
		}
	}
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueIsOpen", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}
	return false
}

// CheckForPR checks if the issue has an open PR
func (r *GithubIssueReconciler) CheckForPR(githubIssue *github.Issue, issueObject *issues.GithubIssue) bool {
	if githubIssue == nil {
		return false
	}
	condition := &metav1.Condition{
		Type:    "IssueHasPR",
		Status:  metav1.ConditionFalse,
		Reason:  "IssueHasNoPR",
		Message: "Issue has no PR",
	}
	if githubIssue.GetPullRequestLinks() != nil {
		condition = &metav1.Condition{
			Type:    "IssueHasPR",
			Status:  metav1.ConditionTrue,
			Reason:  "IssueHasPR",
			Message: "Issue has an open PR",
		}
	}
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueHasPR", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}
	return false
}

// fetchAllIssues gets all issues in a repository with retry for rate limits
func (r *GithubIssueReconciler) fetchAllIssues(ctx context.Context, owner string, repo string) ([]*github.Issue, error) {
	const maxRetries = 5
	const baseDelay = time.Second

	var backoffDelay time.Duration
	for attempt := 1; attempt <= maxRetries; attempt++ {
		allIssues, err := r.IssueClient.List(ctx, owner, repo)
		if err == nil {
			r.Log.Info("Fetched issues successfully")
			return allIssues, nil
		}

		if attempt < maxRetries {
			backoffDelay = baseDelay * (1 << (attempt - 1)) // Exponential backoff (2^n-1)
			r.Log.Warn(fmt.Sprintf("Attempt %d failed. Retrying after %v due to error: %v", attempt, backoffDelay, err))
			time.Sleep(backoffDelay)
		}
	}

	return nil, fmt.Errorf("exceeded retries fetching issues")
}

// CloseIssue closes the issue on GitHub Repo.
func (r *GithubIssueReconciler) CloseIssue(ctx context.Context, owner string, repo string, gitHubIssue *github.Issue) error {
	if gitHubIssue == nil {
		return fmt.Errorf("cannot close issue: issue is nil")
	}
	closedIssue, err := r.IssueClient.Close(ctx, owner, repo, *gitHubIssue.Number)
	if err != nil {
		return fmt.Errorf("failed to close issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Closed issue: %s", closedIssue.GetHTMLURL()))
	return nil
}

// CreateIssue creates a new issue in the repository
func (r *GithubIssueReconciler) CreateIssue(ctx context.Context, owner string, repo string, issueObject *issues.GithubIssue) error {
	createdIssue, err := r.IssueClient.Create(ctx, owner, repo, issueObject.Spec.Title, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to create issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Created GitHub issue: %s", createdIssue.GetHTMLURL()))
	return nil
}

// EditIssue edits the description of an existing issue in the repository
func (r *GithubIssueReconciler) EditIssue(ctx context.Context, owner string, repo string, issueObject *issues.GithubIssue, issueNumber int) error {
	issue, err := r.IssueClient.Edit(ctx, owner, repo, issueNumber, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to edit issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Edited issue: %s", issue.GetHTMLURL()))
	return nil
}

// FindIssue finds a specific issue in the repository by title
func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner, repo string, issue *issues.GithubIssue) (*github.Issue, error) {
	allIssues, err := r.fetchAllIssues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}
	return searchForIssue(issue, allIssues), nil
}

// ParseRepoURL parses a repository URL and extracts the owner and repository name.
// Returns an error if the URL format is invalid.
func ParseRepoURL(repoURL string) (string, string, error) {
	parts := strings.Split(repoURL, "/")
	if len(parts) < 5 {
		return "", "", fmt.Errorf("invalid repository URL: %s", repoURL)
	}
	return parts[3], parts[4], nil
}
