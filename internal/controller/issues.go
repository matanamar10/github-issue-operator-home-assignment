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
	if githubIssue == nil {
		return fmt.Errorf("githubIssue is nil")
	}

	PRChange := r.CheckForPr(githubIssue, issue)
	OpenChange := r.CheckIfOpen(githubIssue, issue)

	if PRChange || OpenChange {
		r.Log.Info("Updating Issue status")
		err := r.Client.Status().Update(ctx, issue)
		if err != nil {
			if fallbackErr := r.Client.Update(ctx, issue); fallbackErr != nil {
				r.Recorder.Event(issue, corev1.EventTypeWarning, "StatusUpdateFailed", fmt.Sprintf("Failed to update status: %v", fallbackErr))
				return fmt.Errorf("unable to update status: %v", fallbackErr)
			}
		}
		r.Recorder.Event(issue, corev1.EventTypeNormal, "StatusUpdated", "Issue status updated")
		r.Log.Info("Issue status updated successfully")
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

// CheckForPr checks if the issue has an open PR
func (r *GithubIssueReconciler) CheckForPr(githubIssue *github.Issue, issueObject *issues.GithubIssue) bool {
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
	opt := &github.IssueListByRepoOptions{}
	maxRetries := 5

	for attempt := 1; attempt <= maxRetries; attempt++ {
		allIssues, response, err := r.GitHubClient.Issues.ListByRepo(ctx, owner, repo, opt)
		if err == nil {
			r.Log.Info("Fetched issues successfully")
			return allIssues, nil
		}
		if response != nil && response.StatusCode == 403 {
			resetTime := response.Rate.Reset.Time
			waitDuration := time.Until(resetTime) + time.Second
			r.Log.Warn(fmt.Sprintf("Rate limit hit. Retrying after %v seconds.", waitDuration.Seconds()))
			time.Sleep(waitDuration)
			continue
		}
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}
	return nil, fmt.Errorf("exceeded retries fetching issues")
}

// CloseIssue closes the issue on GitHub
func (r *GithubIssueReconciler) CloseIssue(ctx context.Context, owner string, repo string, gitHubIssue *github.Issue) error {
	if gitHubIssue == nil {
		return errors.New("could not find issue in repository")
	}
	state := "closed"
	closedIssueRequest := &github.IssueRequest{State: &state}
	_, _, err := r.GitHubClient.Issues.Edit(ctx, owner, repo, *gitHubIssue.Number, closedIssueRequest)
	if err != nil {
		return fmt.Errorf("failed to close GitHub issue: %v", err)
	}
	r.Log.Info(fmt.Sprintf("Closed GitHub issue: %s", gitHubIssue.GetHTMLURL()))
	return nil
}

// CreateIssue creates a new issue in the repository
func (r *GithubIssueReconciler) CreateIssue(ctx context.Context, owner string, repo string, issueObject *issues.GithubIssue) error {
	newIssue := &github.IssueRequest{Title: &issueObject.Spec.Title, Body: &issueObject.Spec.Description}
	createdIssue, response, err := r.GitHubClient.Issues.Create(ctx, owner, repo, newIssue)
	if err != nil {
		if response != nil {
			return fmt.Errorf("failed to create issue: %s, %v", response.Status, err)
		}
		return fmt.Errorf("failed to create issue: %v", err)
	}
	r.Log.Info(fmt.Sprintf("Created GitHub issue: %s", createdIssue.GetHTMLURL()))
	return nil
}

// EditIssue edits the description of an existing issue in the repository
func (r *GithubIssueReconciler) EditIssue(ctx context.Context, owner string, repo string, issueObject *issues.GithubIssue, issueNumber int) error {
	editIssueRequest := &github.IssueRequest{Body: &issueObject.Spec.Description}
	_, _, err := r.GitHubClient.Issues.Edit(ctx, owner, repo, issueNumber, editIssueRequest)
	if err != nil {
		return fmt.Errorf("failed to edit issue: %v", err)
	}
	return nil
}

// FindIssue finds a specific issue in the repository by title
func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner string, repo string, issue *issues.GithubIssue) (*github.Issue, error) {
	allIssues, err := r.fetchAllIssues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}
	return searchForIssue(issue, allIssues), nil
}
