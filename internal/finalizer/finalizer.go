package finalizer

import (
	"fmt"
	issues "github.com/matanamar10/github-issue-operator-hhome-assignment/api/v1alpha1"
	"go.uber.org/zap"

	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const closeIssueFinalizer = "issues.dana.io/finalizer"

// Ensure adds finalizer to GithubIssue CRD if missing
func Ensure(ctx context.Context, c client.Client, obj client.Object, logger *zap.Logger) error {
	githubIssue, ok := obj.(*issues.GithubIssue)
	if !ok {
		return fmt.Errorf("unexpected type: expected *issues.GithubIssue, got %T", obj)
	}
	if !controllerutil.ContainsFinalizer(obj, closeIssueFinalizer) {
		controllerutil.AddFinalizer(obj, closeIssueFinalizer)
		if err := c.Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to add finalizer: %w", err)
		}
		logger.Info("Finalizer added successfully",
			zap.String("finalizer", closeIssueFinalizer),
			zap.String("githubIssue", githubIssue.Name),
		)
	}
	return nil

}

// Cleanup performs finalizer actions, removing the finalizer from the githubIssue object.
func Cleanup(ctx context.Context, c client.Client, obj client.Object, logger *zap.Logger) error {
	githubIssue, ok := obj.(*issues.GithubIssue)
	if !ok {
		return fmt.Errorf("unexpected type: expected *issues.GithubIssue, got %T", obj)
	}

	logger.Info("Starting cleanup for githubIssue",
		zap.String("githubIssue", githubIssue.Name),
	)

	controllerutil.RemoveFinalizer(obj, closeIssueFinalizer)
	if err := c.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to remove finalizer: %w", err)
	}

	logger.Info("Finalizer added successfully",
		zap.String("finalizer", closeIssueFinalizer),
		zap.String("githubIssue", githubIssue.Name),
	)
	return nil
}
