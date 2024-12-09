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
	"net/http"

	"github.com/google/go-github/v56/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"math/rand"
	"time"

	issuesv1alpha1 "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	. "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
)

func RandomString() string {
	source := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(source)

	length := 8
	charset := "abcdefghijklmnopqrstuvwxyz0123456789"

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}
	return string(b)
}

func GenerateTestIssue() *issuesv1alpha1.GithubIssue {
	name := RandomString()
	title := RandomString()
	description := RandomString()
	newIssue := &issuesv1alpha1.GithubIssue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: issuesv1alpha1.GithubIssueSpec{
			Title:       title,
			Description: description,
			Repo:        "https://github.com/test/test",
		},
	}
	return newIssue
}

func CreateFakeClient(issue *issuesv1alpha1.GithubIssue) (client.Client, *runtime.Scheme, error) {
	obj := []client.Object{issue}
	s := scheme.Scheme
	err := issuesv1alpha1.AddToScheme(s)
	if err != nil {
		return nil, nil, err
	}
	c := NewClientBuilder().WithObjects(obj...).Build()

	return c, s, nil

}

var _ = Describe("githubIssue controller e2e test", func() {
	var (
		timeout  = time.Second * 20
		interval = time.Millisecond * 250
	)
	Context("e2e testing", func() {
		It("creates an issue", func() {
			name := fmt.Sprintf("e2e-test-%s", RandomString())
			testIssue := &issuesv1alpha1.GithubIssue{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: "default",
				},
				Spec: issuesv1alpha1.GithubIssueSpec{
					Title:       name,
					Description: "this is generated from an e2e-test",
					Repo:        "https://github.com/dvirgilad/githubIssue-operator-assignment",
				},
			}
			err := k8sClient.Create(ctx, testIssue)
			Expect(err).ToNot(HaveOccurred())
			githubIssueReconciled := issuesv1alpha1.GithubIssue{}
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testIssue.ObjectMeta.Name,
					Namespace: testIssue.Namespace,
				},
			}

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, req.NamespacedName, &githubIssueReconciled)).Should(BeNil(), "should find resource")
				return meta.IsStatusConditionTrue(githubIssueReconciled.Status.Conditions, "IssueIsOpen")
			}, timeout, interval).Should(BeTrue())
			By("updating issue")
			githubIssueReconciled.Spec.Description = "updated description"
			Expect(k8sClient.Update(ctx, &githubIssueReconciled)).Should(Succeed())

			By("deleting issue")
			Expect(k8sClient.Delete(ctx, &githubIssueReconciled)).Should(Succeed())
			deletedIssue := &issuesv1alpha1.GithubIssue{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, req.NamespacedName, deletedIssue)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})

var _ = Describe("githubIssue controller", func() {
	Context("When creating githubIssue ", func() {
		It("Receive error when trying to create an issue", func() {
			By("create Issue")

			ctx := context.Background()
			testIssue := GenerateTestIssue()
			c, s, err := CreateFakeClient(testIssue)
			Expect(err).To(BeNil())

			MockClient = mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposIssuesByOwnerByRepo,
					[]*github.Issue{
						{
							ID:    github.Int64(123),
							Title: github.String("Issue 1"),
						},
						{
							ID:    github.Int64(456),
							Title: github.String("Issue 2"),
						},
					},
					[]*github.Issue{
						{
							ID:    github.Int64(123),
							Title: github.String("Issue 1"),
						},
						{
							ID:    github.Int64(456),
							Title: github.String("Issue 2"),
						},
					},
				),
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(
							w,
							http.StatusInternalServerError,
							"github went belly up or something",
						)
					}),
				),
			)

			ghClient := github.NewClient(MockClient)
			r := &GithubIssueReconciler{Client: c,
				Scheme: s, Log: TestLog, GitHubClient: ghClient}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testIssue.ObjectMeta.Name,
					Namespace: testIssue.Namespace,
				},
			}

			_, err = r.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())

			githubIssueReconciled := issuesv1alpha1.GithubIssue{}

			err = c.Get(ctx, req.NamespacedName, &githubIssueReconciled)
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.IsStatusConditionTrue(githubIssueReconciled.Status.Conditions, "IssueIsOpen")).To(BeFalse())
		})
	})
})

var _ = Describe("githubIssue controller", func() {
	Context("When creating githubIssue ", func() {
		It("Receive error when trying to update an issue", func() {
			By("editing Issue")

			ctx := context.Background()
			testIssue := GenerateTestIssue()
			c, s, err := CreateFakeClient(testIssue)
			Expect(err).To(BeNil())

			MockClient = mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposIssuesByOwnerByRepo,
					[]*github.Issue{
						{
							ID:     github.Int64(123),
							Number: github.Int(123),
							Title:  github.String(testIssue.Spec.Title),
						},
						{
							ID:     github.Int64(456),
							Number: github.Int(456),
							Title:  github.String("Issue 2"),
						},
					},
					[]*github.Issue{
						{
							ID:     github.Int64(123),
							Number: github.Int(123),
							Title:  github.String(testIssue.Spec.Title),
						},
						{
							ID:     github.Int64(456),
							Number: github.Int(456),
							Title:  github.String("Issue 2"),
						},
					},
				),
				mock.WithRequestMatchHandler(
					mock.PatchReposIssuesByOwnerByRepoByIssueNumber,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mock.WriteError(
							w,
							http.StatusInternalServerError,
							"github went belly up or something",
						)
					}),
				),
			)

			ghClient := github.NewClient(MockClient)
			r := &GithubIssueReconciler{Client: c,
				Scheme: s, Log: TestLog, GitHubClient: ghClient}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testIssue.ObjectMeta.Name,
					Namespace: testIssue.Namespace,
				},
			}

			_, err = r.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())

			githubIssueReconciled := issuesv1alpha1.GithubIssue{}

			err = c.Get(ctx, req.NamespacedName, &githubIssueReconciled)
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.IsStatusConditionTrue(githubIssueReconciled.Status.Conditions, "IssueIsOpen")).To(BeFalse())
		})
	})
})
