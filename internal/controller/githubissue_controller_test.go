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
	"encoding/json"
	"fmt"
	"github.com/google/go-github/v56/github"
	"net/http"

	"github.com/migueleliasweb/go-github-mock/src/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"

	"math/rand"
	"time"

	issuesv1alpha1 "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// RandomString generates a random string of length 8
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

// GenerateTestIssue generates a random test issue
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

var (
	timeout  = time.Second * 20
	interval = time.Millisecond * 250
)

var _ = Describe("githubIssue controller", func() {
	Context("e2e testing", func() {
		It("creates an issue", func() {
			name := fmt.Sprintf("e2e-test-%s", RandomString())
			testIssue := &issuesv1alpha1.GithubIssue{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
				},
				Spec: issuesv1alpha1.GithubIssueSpec{
					Title:       name,
					Description: "this is generated from an e2e-test",
					Repo:        "https://github.com/matanamar10/github-issue-operator-home-assignment",
				},
			}
			err := k8sClient.Create(ctx, testIssue)
			Expect(err).ToNot(HaveOccurred())

			githubIssueReconciled := issuesv1alpha1.GithubIssue{}
			req := types.NamespacedName{
				Name:      testIssue.ObjectMeta.Name,
				Namespace: testIssue.Namespace,
			}

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, req, &githubIssueReconciled)).Should(BeNil(), "should find resource")
				return meta.IsStatusConditionTrue(githubIssueReconciled.Status.Conditions, "IssueIsOpen")
			}, timeout, interval).Should(BeTrue())

			By("updating issue")
			githubIssueReconciled.Spec.Description = "updated description"
			Expect(k8sClient.Update(ctx, &githubIssueReconciled)).Should(Succeed())

			By("deleting issue")
			Expect(k8sClient.Delete(ctx, &githubIssueReconciled)).Should(Succeed())

			deletedIssue := &issuesv1alpha1.GithubIssue{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, req, deletedIssue)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})

var _ = Describe("githubIssue controller", func() {
	Context("When creating githubIssue", func() {
		It("Receive error when trying to create an issue", func() {
			By("create Issue")

			testIssue := GenerateTestIssue()

			MockClient = mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						http.Error(w, "github went belly up or something", http.StatusInternalServerError)
					}),
				),
			)

			req := types.NamespacedName{
				Name:      testIssue.ObjectMeta.Name,
				Namespace: testIssue.Namespace,
			}

			Expect(k8sClient.Create(ctx, testIssue)).To(Succeed())

			Eventually(func() bool {
				updatedIssue := &issuesv1alpha1.GithubIssue{}
				err := k8sClient.Get(ctx, req, updatedIssue)
				return err == nil && meta.IsStatusConditionTrue(updatedIssue.Status.Conditions, "IssueIsOpen") == false
			}, timeout, interval).Should(BeTrue())
		})

		It("should create a new issue successfully if issue does not exist", func() {
			By("creating a new issue")

			testIssue := GenerateTestIssue()

			MockClient = mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						response := &github.Issue{
							ID:     github.Int64(123),
							Number: github.Int(1),
							Title:  github.String(testIssue.Spec.Title),
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusCreated)
						err := json.NewEncoder(w).Encode(response)
						if err != nil {
							return
						}
					}),
				),
			)

			req := types.NamespacedName{
				Name:      testIssue.ObjectMeta.Name,
				Namespace: testIssue.Namespace,
			}

			Expect(k8sClient.Create(ctx, testIssue)).To(Succeed())

			Eventually(func() bool {
				updatedIssue := &issuesv1alpha1.GithubIssue{}
				err := k8sClient.Get(ctx, req, updatedIssue)
				return err == nil && meta.IsStatusConditionTrue(updatedIssue.Status.Conditions, "IssueIsOpen")
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When updating githubIssue", func() {
		It("Receive error when trying to update an issue", func() {
			By("update Issue")

			testIssue := GenerateTestIssue()

			MockClient = mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						http.Error(w, "github issue update failed", http.StatusInternalServerError)
					}),
				),
			)

			req := types.NamespacedName{
				Name:      testIssue.ObjectMeta.Name,
				Namespace: testIssue.Namespace,
			}

			Expect(k8sClient.Create(ctx, testIssue)).To(Succeed())

			testIssue.Spec.Description = "Updated Description"
			Expect(k8sClient.Update(ctx, testIssue)).To(Succeed())

			Eventually(func() bool {
				updatedIssue := &issuesv1alpha1.GithubIssue{}
				err := k8sClient.Get(ctx, req, updatedIssue)
				return err == nil && meta.IsStatusConditionTrue(updatedIssue.Status.Conditions, "IssueIsOpen") == false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When deleting githubIssue", func() {
		It("should close the issue successfully", func() {
			By("deleting Issue")

			testIssue := GenerateTestIssue()

			MockClient = mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Return a mocked existing issue
						response := &github.Issue{
							ID:     github.Int64(123),
							Number: github.Int(1),
							State:  github.String("open"),
							Title:  github.String(testIssue.Spec.Title),
						}
						w.Header().Set("Content-Type", "application/json")
						err := json.NewEncoder(w).Encode(response)
						if err != nil {
							return
						}
					}),
				),
				mock.WithRequestMatchHandler(
					mock.PostReposIssuesByOwnerByRepo,
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						response := &github.Issue{
							State: github.String("closed"),
						}
						w.Header().Set("Content-Type", "application/json")
						err := json.NewEncoder(w).Encode(response)
						if err != nil {
							return
						}
					}),
				),
			)

			req := types.NamespacedName{
				Name:      testIssue.ObjectMeta.Name,
				Namespace: testIssue.Namespace,
			}

			Expect(k8sClient.Create(ctx, testIssue)).To(Succeed())

			Expect(k8sClient.Delete(ctx, testIssue)).To(Succeed())

			Eventually(func() bool {
				updatedIssue := &issuesv1alpha1.GithubIssue{}
				err := k8sClient.Get(ctx, req, updatedIssue)
				return err == nil && meta.IsStatusConditionTrue(updatedIssue.Status.Conditions, "IssueIsOpen") == false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
