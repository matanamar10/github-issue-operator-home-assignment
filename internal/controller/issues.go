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

// updateIssueCondition updates the status condition of the GitHub issue if necessary
func updateIssueCondition(issueObject *issuesv1alpha1.GithubIssue, condition *metav1.Condition) bool {
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueIsOpen", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}
	return false
}

// checkIfOpen checks if the issue is open and returns the corresponding condition
func checkIfOpen(platformIssue *git.Issue) (*metav1.Condition, bool) {
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

// Helper function to check if an issue exists.
func issueExists(issue *git.Issue) bool {
	return issue != nil
}
