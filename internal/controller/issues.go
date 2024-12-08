package controller

import (
	"context"
	"fmt"
	issuesv1alpha1 "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	"github.com/matanamar10/github-issue-operator-hhome-assignment/internal/finalizer"
	"github.com/matanamar10/github-issue-operator-hhome-assignment/internal/git"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
	"time"
)

// searchForIssue checks if the generic Issue list contains an issue matching the specified CRD.
func searchForIssue(issueTitle string, platformIssues []*git.Issue) *git.Issue {
	for _, platformIssue := range platformIssues {
		if platformIssue != nil && platformIssue.Title == issueTitle {
			return platformIssue
		}
	}
	return nil
}

// updateIssueCondition updates the status condition of the GitHub issue if necessary
func updateIssueCondition(issueObject *issuesv1alpha1.GithubIssue, condition *metav1.Condition) bool {
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueIsOpen", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}
	return false
}

// checkIfOpen checks if the issue is open and returns the corresponding condition
func (r *GithubIssueReconciler) checkIfOpen(platformIssue *git.Issue) (*metav1.Condition, bool) {
	if platformIssue == nil {
		return nil, false
	}

	state := platformIssue.State
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

	return condition, true
}

func checkForPR(platformIssue *git.Issue, issueObject *issuesv1alpha1.GithubIssue) bool {
	if platformIssue == nil {
		return false
	}

	var condition *metav1.Condition
	if platformIssue.HasPR {
		condition = &metav1.Condition{
			Type:    "IssueHasPR",
			Status:  metav1.ConditionTrue,
			Reason:  "IssueHasPR",
			Message: "Issue has an associated PR",
		}
	} else {
		condition = &metav1.Condition{
			Type:    "IssueHasPR",
			Status:  metav1.ConditionFalse,
			Reason:  "IssueHasNoPR",
			Message: "Issue has no PR",
		}
	}

	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueHasPR", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}

	return false
}

func (r *GithubIssueReconciler) fetchAllIssues(ctx context.Context, owner, repo string) ([]*git.Issue, error) {
	var allIssues []*git.Issue

	backoff := wait.Backoff{
		Duration: time.Second,
		Factor:   2.0,
		Steps:    5,
	}

	err := retry.OnError(backoff, r.shouldRetry, func() error {
		return r.fetchIssuesFromGitHub(ctx, owner, repo, &allIssues)
	})

	if err != nil {
		return nil, fmt.Errorf("exceeded retries fetching issues: %w", err)
	}

	r.Log.Info("Fetched issues successfully")
	return allIssues, nil
}

// shouldRetry defines the condition for retrying (retry on any error)
func (r *GithubIssueReconciler) shouldRetry(err error) bool {
	// Log the error that is causing the retry
	if err != nil {
		r.Log.Warn("Retrying after error", zap.Error(err))
	}
	return true // Retry on any error
}

// fetchIssuesFromGitHub fetches issues from GitHub and updates the allIssues slice
func (r *GithubIssueReconciler) fetchIssuesFromGitHub(ctx context.Context, owner, repo string, allIssues *[]*git.Issue) error {
	fetchedIssues, fetchErr := r.IssueClient.List(ctx, owner, repo)
	if fetchErr != nil {
		r.Log.Warn("Failed to fetch issues, retrying", zap.Error(fetchErr))
		return fetchErr
	}

	*allIssues = fetchedIssues
	return nil
}

// CloseIssue closes the issue on GitHub Repo.
func (r *GithubIssueReconciler) CloseIssue(ctx context.Context, owner, repo string, platformIssue *git.Issue) error {
	if platformIssue == nil {
		return fmt.Errorf("cannot close issue: issue is nil")
	}

	closedIssue, err := r.IssueClient.Close(ctx, owner, repo, platformIssue.Number)
	if err != nil {
		return fmt.Errorf("failed to close issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Closed issue: %s", closedIssue.URL))
	return nil
}

// CreateIssue creates a new issue in the repository.
func (r *GithubIssueReconciler) CreateIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue) error {
	createdIssue, err := r.IssueClient.Create(ctx, owner, repo, issueObject.Spec.Title, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to create issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Created issue: %s", createdIssue.URL))
	return nil
}

// EditIssue edits the description of an existing issue in the repository.
func (r *GithubIssueReconciler) EditIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue, issueNumber int) error {
	editedIssue, err := r.IssueClient.Edit(ctx, owner, repo, issueNumber, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to edit issue: %v", err)
	}

	r.Log.Info(fmt.Sprintf("Edited issue: %s", editedIssue.URL))
	return nil
}

// FindIssue finds a specific issue in the repository by title.
func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner, repo string, issue *issuesv1alpha1.GithubIssue) (*git.Issue, error) {
	allIssues, err := r.fetchAllIssues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}

	return searchForIssue(issue.Spec.Title, allIssues), nil
}

// parseRepoURL parses a repository URL and extracts the owner and repository name.
// Returns an error if the URL format is invalid.
func parseRepoURL(repoURL string) (string, string, error) {
	parts := strings.Split(repoURL, "/")
	if len(parts) < 5 {
		return "", "", fmt.Errorf("invalid repository URL: %s", repoURL)
	}
	return parts[3], parts[4], nil
}

// handleNewIssue function manage a creation of new issue.
func (r *GithubIssueReconciler) handleNewIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue) (ctrl.Result, error) {
	r.Log.Info("Creating new issue")

	if err := r.CreateIssue(ctx, owner, repo, issueObject); err != nil {
		r.Log.Error("Failed to create issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	issue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		r.Log.Error("Failed to fetch newly created issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	if issueExists(issue) {
		if err := r.updateIssueStatus(ctx, issueObject, issue); err != nil {
			r.Log.Error("Failed to update issue status", zap.Error(err))
		}
	} else {
		r.Log.Warn("Cannot update status: issue is nil", zap.String("IssueName", issueObject.Name), zap.String("Namespace", issueObject.Namespace))
	}

	r.Log.Info("Issue created successfully")
	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) handleUpdatedIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue, issue *git.Issue) (ctrl.Result, error) {
	r.Log.Info("Editing issue")

	if err := r.EditIssue(ctx, owner, repo, issueObject, issue.Number); err != nil {
		r.Log.Error("Failed to edit issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	updatedIssue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		r.Log.Error("Failed to fetch updated issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	if issueExists(updatedIssue) {
		if err := r.updateIssueStatus(ctx, issueObject, updatedIssue); err != nil {
			r.Log.Error("Failed to update issue status", zap.Error(err))
		}
	} else {
		r.Log.Warn("Cannot update status: issue is nil", zap.String("IssueName", issueObject.Name), zap.String("Namespace", issueObject.Namespace))
	}

	r.Log.Info("Issue edited successfully")
	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) handleDeletion(ctx context.Context, owner, repo string, issue *git.Issue, issueObject *issuesv1alpha1.GithubIssue) (ctrl.Result, error) {
	r.Log.Info("Closing issue")

	if !issueExists(issue) {
		return ctrl.Result{}, fmt.Errorf("cannot close issue: issue is nil")
	}

	if err := r.CloseIssue(ctx, owner, repo, issue); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed closing issue: %v", err)
	}

	if err := finalizer.Cleanup(ctx, r.Client, issueObject, r.Log); err != nil {
		r.Log.Error("Failed cleaning up finalizer", zap.Error(err))
		return ctrl.Result{}, err
	}

	r.Log.Info("Issue closed and finalizer cleaned up successfully")
	return ctrl.Result{}, nil
}

// Helper function to check if an issue exists.
func issueExists(issue *git.Issue) bool {
	return issue != nil
}
