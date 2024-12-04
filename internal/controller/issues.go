package controller

import (
	"context"
	"fmt"
	"github.com/google/go-github/v56/github"
	issuesv1alpha1 "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	"github.com/matanamar10/github-issue-operator-hhome-assignment/internal/finalizer"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
	"time"
)

// searchForIssue checks if GithubIssue CRD has an issue in the repo
func searchForIssue(issue *issuesv1alpha1.GithubIssue, gitHubIssues []*github.Issue) *github.Issue {
	for _, ghIssue := range gitHubIssues {
		if ghIssue != nil && strings.EqualFold(*ghIssue.Title, issue.Spec.Title) {
			return ghIssue
		}
	}
	return nil
}

// UpdateIssueStatus updates the status of the GithubIssue CRD
func (r *GithubIssueReconciler) UpdateIssueStatus(ctx context.Context, issue *issuesv1alpha1.GithubIssue, githubIssue *github.Issue) error {
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
func (r *GithubIssueReconciler) CheckIfOpen(githubIssue *github.Issue, issueObject *issuesv1alpha1.GithubIssue) bool {
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
func (r *GithubIssueReconciler) CheckForPR(githubIssue *github.Issue, issueObject *issuesv1alpha1.GithubIssue) bool {
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
func (r *GithubIssueReconciler) CreateIssue(ctx context.Context, owner string, repo string, issueObject *issuesv1alpha1.GithubIssue) error {
	createdIssue, err := r.IssueClient.Create(ctx, owner, repo, issueObject.Spec.Title, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to create issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Created GitHub issue: %s", createdIssue.GetHTMLURL()))
	return nil
}

// EditIssue edits the description of an existing issue in the repository
func (r *GithubIssueReconciler) EditIssue(ctx context.Context, owner string, repo string, issueObject *issuesv1alpha1.GithubIssue, issueNumber int) error {
	issue, err := r.IssueClient.Edit(ctx, owner, repo, issueNumber, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to edit issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Edited issue: %s", issue.GetHTMLURL()))
	return nil
}

// FindIssue finds a specific issue in the repository by title
func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner, repo string, issue *issuesv1alpha1.GithubIssue) (*github.Issue, error) {
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

func (r *GithubIssueReconciler) handleNewIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue) (ctrl.Result, error) {
	r.Log.Info("creating issue")
	if err := r.CreateIssue(ctx, owner, repo, issueObject); err != nil {
		r.Log.Error("failed creating issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	issue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		r.Log.Error("failed fetching newly created issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	if issueExists(issue) {
		if err := r.UpdateIssueStatus(ctx, issueObject, issue); err != nil {
			r.Log.Error("failed updating issue status", zap.Error(err))
		}
	} else {
		r.Log.Warn("Cannot update status: issue is nil", zap.String("IssueName", issueObject.Name), zap.String("Namespace", issueObject.Namespace))
	}

	r.Log.Info("issue created successfully")
	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) handleUpdatedIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue, issue *github.Issue) (ctrl.Result, error) {
	r.Log.Info("editing issue")
	if err := r.EditIssue(ctx, owner, repo, issueObject, *issue.Number); err != nil {
		r.Log.Error("failed editing issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	updatedIssue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		r.Log.Error("failed fetching updated issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	if issueExists(updatedIssue) {
		if err := r.UpdateIssueStatus(ctx, issueObject, updatedIssue); err != nil {
			r.Log.Error("failed updating issue status", zap.Error(err))
		}
	} else {
		r.Log.Warn("Cannot update status: issue is nil", zap.String("IssueName", issueObject.Name), zap.String("Namespace", issueObject.Namespace))
	}

	r.Log.Info("issue edited successfully")
	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) handleDeletion(ctx context.Context, owner, repo string, issue *github.Issue, issueObject *issuesv1alpha1.GithubIssue) (ctrl.Result, error) {
	r.Log.Info("closing issue")
	if !issueExists(issue) {
		return ctrl.Result{}, fmt.Errorf("cannot close issue: issue is nil")
	}

	if err := r.CloseIssue(ctx, owner, repo, issue); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed closing issue: %v", err)
	}

	if err := finalizer.Cleanup(ctx, r.Client, issueObject, r.Log); err != nil {
		r.Log.Error("failed cleaning up finalizer", zap.Error(err))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Helper function to check if an issue exists.
func issueExists(issue *github.Issue) bool {
	return issue != nil
}
