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

	owner, repo, err := ParseRepoURL(issueObject.Spec.Repo)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed parse repoURL : %v", err)
	}

	log.Info(fmt.Sprintf("attempting to get issues from %s/%s", owner, repo))
	gitHubIssue, err := r.FindIssue(ctx, owner, repo, issueObject)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !issueObject.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, owner, repo, gitHubIssue, issueObject)
	}
	err = finalizer.Ensure(ctx, r.Client, issueObject, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !issueExists(gitHubIssue) {
		return r.handleNewIssue(ctx, owner, repo, issueObject)
	} else {

		return r.handleUpdatedIssue(ctx, owner, repo, issueObject, gitHubIssue)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&issuesv1alpha1.GithubIssue{}).
		Complete(r)
}
