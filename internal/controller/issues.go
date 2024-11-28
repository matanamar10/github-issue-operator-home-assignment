package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/go-github/v56/github"
	issues "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// Checks if GithubIssue CRD has an issue in the repo
func searchForIssue(issue *issues.GithubIssue, gitHubIssues []*github.Issue) *github.Issue {
	for _, ghIssue := range gitHubIssues {
		if strings.EqualFold(*ghIssue.Title, issue.Spec.Title) {

			return ghIssue
		}
	}
	return nil
}

// UpdateIssueStatus updates the status of the GithubIssue CRD
func (r *GithubIssueReconciler) UpdateIssueStatus(ctx context.Context, issue *issues.GithubIssue, githubIssue *github.Issue) error {
	PRChange := r.CheckForPr(githubIssue, issue)
	OpenChange := r.CheckIfOpen(githubIssue, issue)

	if OpenChange || PRChange {
		r.Log.Info("editing Issue status")
		err := r.Client.Status().Update(ctx, issue)
		if err != nil {
			if err := r.Client.Update(ctx, issue); err != nil {
				r.Recorder.Event(issue, corev1.EventTypeWarning, "StatusUpdateFailed", fmt.Sprintf("Failed to update status of CR: %v", err.Error()))
				return fmt.Errorf("unable to update status of CR: %v", err.Error())
			}
		}
		r.Recorder.Event(issue, corev1.EventTypeNormal, "StatusUpdated", "Updated the status of the issue")
		r.Log.Info("updated Issue status")
		return nil
	}
	return nil
}

// CheckIfOpen check if issue is open
func (r *GithubIssueReconciler) CheckIfOpen(githubIssue *github.Issue, issueObject *issues.GithubIssue) bool {
	condition := &metav1.Condition{Type: "IssueIsOpen", Status: metav1.ConditionTrue, Reason: "IssueIsOpen", Message: "Issue is open"}
	if state := githubIssue.GetState(); state != "open" {
		condition = &metav1.Condition{Type: "IssueIsOpen", Status: metav1.ConditionFalse, Reason: fmt.Sprintf("Issueis%s", state), Message: fmt.Sprintf("Issue is %s", state)}
	}
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueIsOpen", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}
	return false
}

// CheckForPr check if issue has an open PR
func (r *GithubIssueReconciler) CheckForPr(githubIssue *github.Issue, issueObject *issues.GithubIssue) bool {
	condition := &metav1.Condition{Type: "IssueHasPR", Status: metav1.ConditionFalse, Reason: "IssueHasnopr", Message: "Issue has no pr"}
	if githubIssue.GetPullRequestLinks() != nil {
		condition = &metav1.Condition{Type: "IssueHasPR", Status: metav1.ConditionTrue, Reason: "IssueHasPR", Message: "Issue Has an open PR"}
	}
	if !meta.IsStatusConditionPresentAndEqual(issueObject.Status.Conditions, "IssueHasPR", condition.Status) {
		meta.SetStatusCondition(&issueObject.Status.Conditions, *condition)
		return true
	}
	return false
}

// fetchAllIssues gets all issues in repo
func (r *GithubIssueReconciler) fetchAllIssues(ctx context.Context, owner string, repo string) ([]*github.Issue, error) {
	opt := &github.IssueListByRepoOptions{}
	allIssues, response, err := r.GitHubClient.Issues.ListByRepo(ctx, owner, repo, opt)
	if err != nil {
		if response != nil {
			r.Recorder.Event(nil, corev1.EventTypeWarning, "FetchFailed", fmt.Sprintf("Failed to fetch issues from GitHub: %s", response.Status))
			return []*github.Issue{}, fmt.Errorf("got bad response from GitHub: %s: %v", response.Status, err.Error())
		}
		r.Recorder.Event(nil, corev1.EventTypeWarning, "FetchFailed", "Failed to fetch issues from GitHub")
		return []*github.Issue{}, fmt.Errorf("failed fetching issues: %v", err.Error())
	}
	r.Log.Info("fetched issues")
	return allIssues, nil
}

// CloseIssue closes the issue on GitHub
func (r *GithubIssueReconciler) CloseIssue(ctx context.Context, owner string, repo string, gitHubIssue *github.Issue) error {
	if gitHubIssue == nil {
		err := errors.New("could not find issue in repo")
		r.Recorder.Event(nil, corev1.EventTypeWarning, "CloseFailed", "Failed to close GitHub issue: issue not found")
		return err
	}
	state := "closed"
	closedIssueRequest := &github.IssueRequest{State: &state}
	_, _, err := r.GitHubClient.Issues.Edit(ctx, owner, repo, *gitHubIssue.Number, closedIssueRequest)
	if err != nil {
		r.Recorder.Event(nil, corev1.EventTypeWarning, "CloseFailed", fmt.Sprintf("Failed to close GitHub issue: %v", err.Error()))
		return errors.New("could not close issue")
	}
	r.Recorder.Event(nil, corev1.EventTypeNormal, "Closed", fmt.Sprintf("Closed GitHub issue: %s", gitHubIssue.GetHTMLURL()))
	return nil
}

// CreateIssue add an issue to the repo
func (r *GithubIssueReconciler) CreateIssue(ctx context.Context, owner string, repo string, issueObject *issues.GithubIssue) error {
	newIssue := &github.IssueRequest{Title: &issueObject.Spec.Title, Body: &issueObject.Spec.Description}
	createdIssue, response, err := r.GitHubClient.Issues.Create(ctx, owner, repo, newIssue)
	if err != nil {
		if response != nil {
			r.Recorder.Event(issueObject, corev1.EventTypeWarning, "CreateFailed", fmt.Sprintf("Failed to create GitHub issue: %v", err.Error()))
			return fmt.Errorf("failed creating issue: status %s: %v", response.Status, err.Error())
		} else {
			r.Recorder.Event(issueObject, corev1.EventTypeWarning, "CreateFailed", "Failed to create GitHub issue")
			return fmt.Errorf("failed creating issue: %v", err.Error())
		}
	}
	r.Recorder.Event(issueObject, corev1.EventTypeNormal, "Created", fmt.Sprintf("Created GitHub issue: %s", createdIssue.GetHTMLURL()))
	return nil
}

// EditIssue change the description of an existing issue in the repo
func (r *GithubIssueReconciler) EditIssue(ctx context.Context, owner string, repo string, issueObject *issues.GithubIssue, issueNumber int) error {
	editIssueRequest := &github.IssueRequest{Body: &issueObject.Spec.Description}
	_, response, err := r.GitHubClient.Issues.Edit(ctx, owner, repo, issueNumber, editIssueRequest)
	if err != nil {
		if response != nil {

			return fmt.Errorf("failed editing issue: %v", err.Error())
		}
		return fmt.Errorf("failed editing issue: status %s: %v", response.Status, err.Error())

	}
	return nil
}

func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner string, repo string, issue *issues.GithubIssue) (*github.Issue, error) {
	allIssues, err := r.fetchAllIssues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("falied fetching error: %v", err.Error())
	}
	return searchForIssue(issue, allIssues), nil
}
