package controllers

import (
	"context"
	"testing"

	geov1alpha1 "github.com/oiviadesu/oiviak3s-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDesiredReplicasDefaultsToOne(t *testing.T) {
	t.Parallel()

	if got := desiredReplicas(nil); got != 1 {
		t.Fatalf("expected default desired replicas to be 1, got %d", got)
	}

	two := int32(2)
	if got := desiredReplicas(&two); got != 2 {
		t.Fatalf("expected explicit desired replicas to be preserved, got %d", got)
	}
}

func TestNormalizeSharedEndpointTargetsRejectsDuplicates(t *testing.T) {
	t.Parallel()

	_, err := normalizeSharedEndpointTargets(geov1alpha1.SharedEndpointConfig{
		Enabled: true,
		Endpoints: []geov1alpha1.SharedEndpointTarget{
			{Name: "VN", IP: "192.168.86.8"},
			{Name: "vn", IP: "192.168.86.40"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate endpoint names to fail validation")
	}

	_, err = normalizeSharedEndpointTargets(geov1alpha1.SharedEndpointConfig{
		Enabled: true,
		Endpoints: []geov1alpha1.SharedEndpointTarget{
			{Name: "vn", IP: "192.168.86.8"},
			{Name: "au", IP: "192.168.86.8"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate endpoint IPs to fail validation")
	}
}

func TestBuildSharedEndpointServicesMirrorsPortsAcrossEndpoints(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := geov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add geo scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "immich-gateway", Namespace: "immich"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "immich-gateway"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "immich-gateway"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "gateway",
					Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 80}, {Name: "https", ContainerPort: 443}},
				}}},
			},
		},
	}

	reconciler := &RegionalWorkloadReconciler{
		Client: ctrlclientfake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build(),
		Scheme: scheme,
	}

	workload := &geov1alpha1.RegionalWorkload{
		ObjectMeta: metav1.ObjectMeta{Name: "immich-gateway", Namespace: "immich"},
		Spec: geov1alpha1.RegionalWorkloadSpec{
			WorkloadRef: corev1.ObjectReference{Kind: "Deployment", Name: "immich-gateway"},
		},
	}

	services, err := reconciler.buildSharedEndpointServices(context.Background(), workload, geov1alpha1.SharedEndpointConfig{
		Enabled: true,
		Mode:    "kube-vip",
		Endpoints: []geov1alpha1.SharedEndpointTarget{
			{Name: "vn", IP: "192.168.86.8"},
			{Name: "au", IP: "192.168.86.40"},
		},
	})
	if err != nil {
		t.Fatalf("build services: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	if services[0].Spec.LoadBalancerIP == services[1].Spec.LoadBalancerIP {
		t.Fatal("expected unique load balancer IPs per mirrored endpoint")
	}
	if services[0].Name == services[1].Name {
		t.Fatal("expected unique service names for mirrored endpoints")
	}
	if len(services[0].Spec.Ports) != 2 || len(services[1].Spec.Ports) != 2 {
		t.Fatalf("expected mirrored port sets on both services, got %d and %d", len(services[0].Spec.Ports), len(services[1].Spec.Ports))
	}
	for i := range services[0].Spec.Ports {
		if services[0].Spec.Ports[i] != services[1].Spec.Ports[i] {
			t.Fatalf("expected mirrored port definitions, got %#v and %#v", services[0].Spec.Ports[i], services[1].Spec.Ports[i])
		}
	}
}
