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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GithubIssueSpec defines the desired state of GithubIssue.
type GithubIssueSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https:\/\/[a-zA-Z0-9\-]+(\.[a-zA-Z0-9\-]+)+\/[^\/]+\/[^\/]+$`
	// Repo URL of the repository where the issue should be created
	Repo string `json:"repo,omitempty"`
	// Title is the title of the issue
	Title string `json:"title,omitempty"`
	// Description is used as a description for the issue
	Description string `json:"description,omitempty"`
}

// GithubIssueStatus defines the observed state of GithubIssue.
type GithubIssueStatus struct {
	// Conditions represent the latest available observations of the issue's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GithubIssue is the Schema for the githubissues API.
type GithubIssue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GithubIssueSpec   `json:"spec,omitempty"`
	Status GithubIssueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GithubIssueList contains a list of GithubIssue.
type GithubIssueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GithubIssue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GithubIssue{}, &GithubIssueList{})
}
