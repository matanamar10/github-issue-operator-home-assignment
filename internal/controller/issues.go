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

// UpdateIssueStatus updates the status of the GithubIssue CRD
func (r *GithubIssueReconciler) updateIssueStatus(ctx context.Context, issue *issuesv1alpha1.GithubIssue, platformIssue *git.Issue) error {
	// Check for changes in the issue's PR status and open/closed status
	PRChange := checkForPR(platformIssue, issue)
	OpenChange := r.checkIfOpen(platformIssue, issue)

	if PRChange || OpenChange {
		r.Log.Info("Updating Issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))

		// Attempt to update the CRD's status
		if err := r.Client.Status().Update(ctx, issue); err != nil {
			// Log the error but do not emit an event
			r.Log.Error("Failed to update issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace), zap.Error(err))
			return fmt.Errorf("failed to update status: %v", err)
		}

		// Log success
		r.Log.Info("Issue status updated successfully", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
	} else {
		r.Log.Info("No changes detected in issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
	}

	return nil
}

// CheckIfOpen checks if the issue is open
func (r *GithubIssueReconciler) checkIfOpen(platformIssue *git.Issue, issueObject *issuesv1alpha1.GithubIssue) bool {
	if platformIssue == nil {
		return false
	}

	// Extract the state of the issue
	state := platformIssue.State

	// Prepare the condition for "IssueIsOpen"
	condition := &metav1.Condition{
		Type:    "IssueIsOpen",
		Status:  metav1.ConditionTrue,
		Reason:  "IssueIsOpen",
		Message: "Issue is open",
	}

	// Update the condition if the issue is not open
	if state != "open" {
		condition = &metav1.Condition{
			Type:    "IssueIsOpen",
			Status:  metav1.ConditionFalse,
			Reason:  fmt.Sprintf("IssueIs%s", state),
			Message: fmt.Sprintf("Issue is %s", state),
		}
	}

	// Check if the condition has changed and update if necessary
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueIsOpen", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}

	return false
}

func checkForPR(platformIssue *git.Issue, issueObject *issuesv1alpha1.GithubIssue) bool {
	if platformIssue == nil {
		return false
	}

	// Determine the condition based on whether the issue has an associated PR
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

	// Update the condition if it has changed
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

	// Use retry.OnError to implement exponential backoff retry logic
	err := retry.OnError(backoff, func(err error) bool {
		// Retry on any non-nil error
		return true
	}, func() error {
		var fetchErr error
		allIssues, fetchErr = r.IssueClient.List(ctx, owner, repo)
		if fetchErr != nil {
			r.Log.Warn("Failed to fetch issues, retrying", zap.Error(fetchErr))
		}
		return fetchErr
	})

	if err != nil {
		return nil, fmt.Errorf("exceeded retries fetching issues: %w", err)
	}

	r.Log.Info("Fetched issues successfully")
	return allIssues, nil
}

// CloseIssue closes the issue on GitHub Repo.
func (r *GithubIssueReconciler) CloseIssue(ctx context.Context, owner, repo string, platformIssue *git.Issue) error {
	if platformIssue == nil {
		return fmt.Errorf("cannot close issue: issue is nil")
	}

	// Close the issue using the generic IssueClient
	closedIssue, err := r.IssueClient.Close(ctx, owner, repo, platformIssue.Number)
	if err != nil {
		return fmt.Errorf("failed to close issue: %v", err)
	}

	// Log the URL of the closed issue
	r.Log.Info(fmt.Sprintf("Closed issue: %s", closedIssue.URL))
	return nil
}

// CreateIssue creates a new issue in the repository
func (r *GithubIssueReconciler) CreateIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue) error {
	// Create a new issue using the generic IssueClient
	createdIssue, err := r.IssueClient.Create(ctx, owner, repo, issueObject.Spec.Title, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to create issue: %v", err)
	}

	// Log the URL of the created issue
	r.Log.Info(fmt.Sprintf("Created issue: %s", createdIssue.URL))
	return nil
}

// EditIssue edits the description of an existing issue in the repository
func (r *GithubIssueReconciler) EditIssue(ctx context.Context, owner, repo string, issueObject *issuesv1alpha1.GithubIssue, issueNumber int) error {
	// Edit the issue using the generic IssueClient
	editedIssue, err := r.IssueClient.Edit(ctx, owner, repo, issueNumber, issueObject.Spec.Description)
	if err != nil {
		return fmt.Errorf("failed to edit issue: %v", err)
	}

	// Log the URL of the edited issue
	r.Log.Info(fmt.Sprintf("Edited issue: %s", editedIssue.URL))
	return nil
}

// FindIssue finds a specific issue in the repository by title
func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner, repo string, issue *issuesv1alpha1.GithubIssue) (*git.Issue, error) {
	// Fetch all issues using the generic fetchAllIssues function
	allIssues, err := r.fetchAllIssues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}

	// Extract the title from the GithubIssue object and search for it
	return searchForIssue(issue.Spec.Title, allIssues), nil
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
	r.Log.Info("Creating new issue")

	// Create the new issue using the generic CreateIssue function
	if err := r.CreateIssue(ctx, owner, repo, issueObject); err != nil {
		r.Log.Error("Failed to create issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Find the newly created issue using the generic FindIssue function
	issue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		r.Log.Error("Failed to fetch newly created issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Check if the issue exists and update its status
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

	// Edit the issue using the generic EditIssue function
	if err := r.EditIssue(ctx, owner, repo, issueObject, issue.Number); err != nil {
		r.Log.Error("Failed to edit issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Fetch the updated issue using the generic FindIssue function
	updatedIssue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		r.Log.Error("Failed to fetch updated issue", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Check if the issue exists and update its status
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

	// Check if the issue exists
	if !issueExists(issue) {
		return ctrl.Result{}, fmt.Errorf("cannot close issue: issue is nil")
	}

	// Close the issue using the generic CloseIssue function
	if err := r.CloseIssue(ctx, owner, repo, issue); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed closing issue: %v", err)
	}

	// Cleanup the finalizer
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
