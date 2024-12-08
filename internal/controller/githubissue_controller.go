/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// GithubIssueReconciler reconciles a GithubIssue object
type GithubIssueReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Log         *zap.Logger
	IssueClient git.IssueClient
	Recorder    record.EventRecorder
}

// +kubebuilder:rbac:groups=issues.dana.io,resources=githubissues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=issues.dana.io,resources=githubissues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=issues.dana.io,resources=githubissues/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;watch;list

func (r *GithubIssueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log

	var issueObject = &issuesv1alpha1.GithubIssue{}
	if err := r.Get(ctx, req.NamespacedName, issueObject); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error("unable to fetch issue object", zap.Error(err))
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	owner, repo, err := parseRepoURL(issueObject.Spec.Repo)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed parse repoURL : %v", err)
	}

	log.Info(fmt.Sprintf("attempting to get issues from %s/%s", owner, repo))
	issue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !issueObject.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, owner, repo, issue, issueObject)
	}
	err = finalizer.Ensure(ctx, r.Client, issueObject, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !issueExists(issue) {
		return r.handleNewIssue(ctx, owner, repo, issueObject)
	} else {
		return r.handleUpdatedIssue(ctx, owner, repo, issueObject, issue)
	}
}

// updateIssueStatus updates the status of the GithubIssue CRD
func (r *GithubIssueReconciler) updateIssueStatus(ctx context.Context, issue *issuesv1alpha1.GithubIssue, platformIssue *git.Issue) error {
	conditionType, conditionStatus, reason, message, openChange := checkIfOpen(platformIssue)
	PRChangeConditionType, PRChangeConditionStatus, PRChangeReason, PRChangeMessage, prChange := checkForPR(platformIssue)

	if prChange || openChange {
		r.Log.Info("Updating Issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))

		conditionUpdated := false

		if updateCondition(issue, conditionType, conditionStatus, reason, message) {
			conditionUpdated = true
			r.Log.Info("Condition updated", zap.String("ConditionType", conditionType))
		}

		if updateCondition(issue, PRChangeConditionType, PRChangeConditionStatus, PRChangeReason, PRChangeMessage) {
			conditionUpdated = true
			r.Log.Info("Condition updated", zap.String("ConditionType", PRChangeConditionType))
		}

		if conditionUpdated {
			if err := r.Client.Status().Update(ctx, issue); err != nil {
				r.Log.Error("Failed to update issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace), zap.Error(err))
				return fmt.Errorf("failed to update status: %v", err)
			}
			r.Log.Info("Issue status updated successfully", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
		} else {
			r.Log.Info("No changes detected in conditions", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
		}
	} else {
		r.Log.Info("No changes detected in issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
	}

	return nil
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

// handleUpdatedIssue manage updateing of existing issue.
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

// handleDeletion perform all the needed cleanup logic for issue object.
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

// fetchAllIssues function try to fetch all the issues from the repo.
func (r *GithubIssueReconciler) fetchAllIssues(ctx context.Context, owner, repo string) ([]*git.Issue, error) {
	var allIssues []*git.Issue

	backoff := wait.Backoff{
		Duration: time.Second,
		Factor:   2.0,
		Steps:    5,
	}

	err := retry.OnError(backoff, r.shouldRetry, func() error {
		return r.fetchIssuesFromGit(ctx, owner, repo, &allIssues)
	})

	if err != nil {
		return nil, fmt.Errorf("exceeded retries fetching issues: %w", err)
	}

	r.Log.Info("Fetched issues successfully")
	return allIssues, nil
}

// shouldRetry defines the condition for retrying (retry on any error)
func (r *GithubIssueReconciler) shouldRetry(err error) bool {
	if err != nil {
		r.Log.Warn("Retrying after error", zap.Error(err))
	}
	return true
}

// FindIssue finds a specific issue in the repository by title.
func (r *GithubIssueReconciler) FindIssue(ctx context.Context, owner, repo string, issue *issuesv1alpha1.GithubIssue) (*git.Issue, error) {
	allIssues, err := r.fetchAllIssues(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}

	return searchForIssue(issue.Spec.Title, allIssues), nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&issuesv1alpha1.GithubIssue{}).
		Complete(r)
}
