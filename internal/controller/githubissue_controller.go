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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// UpdateIssueStatus updates the status of the GithubIssue CRD
func (r *GithubIssueReconciler) updateIssueStatus(ctx context.Context, issue *issuesv1alpha1.GithubIssue, platformIssue *git.Issue) error {
	PRChange := checkForPR(platformIssue, issue)
	condition, openChange := checkIfOpen(platformIssue)

	if PRChange || openChange {
		r.Log.Info("Updating Issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))

		if updateIssueCondition(issue, condition) {
			if err := r.Client.Status().Update(ctx, issue); err != nil {
				r.Log.Error("Failed to update issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace), zap.Error(err))
				return fmt.Errorf("failed to update status: %v", err)
			}

			r.Log.Info("Issue status updated successfully", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
		}
	} else {
		r.Log.Info("No changes detected in issue status", zap.String("IssueName", issue.Name), zap.String("Namespace", issue.Namespace))
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&issuesv1alpha1.GithubIssue{}).
		Complete(r)
}
