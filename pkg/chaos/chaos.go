package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Chaos struct {
	Client    client.Client
	Freq      time.Duration
	Namespace string
}

func (c *Chaos) Start(ctx context.Context) {
	ticker := time.NewTicker(c.Freq)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pods, err := c.list(ctx)
			if err != nil {
				fmt.Printf("failed to list pods: %v\n", err)
				continue
			}

			if err := c.delete(ctx, pods[rand.Intn(len(pods))]); err != nil {
				fmt.Printf("failed to delete pod: %v\n", err)
			}
		}
	}
}

func (c *Chaos) list(ctx context.Context) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(c.Namespace),
		client.MatchingLabels{"chaos": "true"},
	}

	if err := c.Client.List(ctx, pods, opts...); err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func (c *Chaos) delete(ctx context.Context, pod corev1.Pod) error {
	fmt.Printf("Deleting pod '%s'\n", pod.Name)
	if err := c.Client.Delete(ctx, &pod, client.GracePeriodSeconds(0)); err != nil {
		return err
	}

	return nil
}
