package controller

import (
	"context"
	"fmt"
	issuesv1alpha1 "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	"github.com/matanamar10/github-issue-operator-hhome-assignment/internal/git"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// updateCondition is a generic function to update any condition of a GitHub issue.
func updateCondition(issueObject *issuesv1alpha1.GithubIssue, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) bool {
	condition := &metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	}

	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, conditionType, condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}

	return false
}

// checkIfOpen checks if the issue is open and returns the corresponding condition
func checkIfOpen(platformIssue *git.Issue) (string, metav1.ConditionStatus, string, string, bool) {
	if platformIssue == nil {
		return "", "", "", "", false
	}

	state := platformIssue.State
	conditionType := "IssueIsOpen"
	conditionStatus := metav1.ConditionTrue
	reason := "IssueIsOpen"
	message := "Issue is open"

	if state != "open" {
		conditionStatus = metav1.ConditionFalse
		reason = fmt.Sprintf("IssueIs%s", state)
		message = fmt.Sprintf("Issue is %s", state)
	}

	return conditionType, conditionStatus, reason, message, true
}

// checkForPR checks if a PR is associated with the issue and returns the condition accordingly
func checkForPR(platformIssue *git.Issue) (string, metav1.ConditionStatus, string, string, bool) {
	if platformIssue == nil {
		return "", "", "", "", false
	}

	conditionType := "IssueHasPR"
	conditionStatus := metav1.ConditionFalse
	reason := "IssueHasNoPR"
	message := "Issue has no PR"

	if platformIssue.HasPR {
		conditionStatus = metav1.ConditionTrue
		reason = "IssueHasPR"
		message = "Issue has an associated PR"
	}

	return conditionType, conditionStatus, reason, message, true
}

// CloseIssue closes the issue on Git Repo.
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

// Helper function to check if an issue exists.
func issueExists(issue *git.Issue) bool {
	return issue != nil
}
