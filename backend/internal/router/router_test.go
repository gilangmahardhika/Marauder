package router_test

import (
	"errors"
	"strings"
	"testing"

	"orchestrator/backend/internal/router"
)

func TestRouteClass(t *testing.T) {
	low, err := router.TaskClass("low", 0)
	if err != nil || low != "implement" {
		t.Fatalf("low 0 = %s %v", low, err)
	}
	high, err := router.TaskClass("high", 0)
	if err != nil || high != "implement_hard" {
		t.Fatalf("high = %s %v", high, err)
	}
	used, err := router.TaskClass("low", 1)
	if err != nil || used != "implement_hard" {
		t.Fatalf("used = %s %v", used, err)
	}
}

func TestNoDeployClass(t *testing.T) {
	if err := router.RejectDeploy("deploy"); err == nil {
		t.Fatal("deploy class was accepted")
	}
	if err := router.RejectDeploy("implement"); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackOnce(t *testing.T) {
	var calls []string
	err := router.Call("primary", "fallback", func(model string) error {
		calls = append(calls, model)
		return errors.New("transport")
	})
	if err == nil || !strings.Contains(err.Error(), "provider_error") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(calls, ",") != "primary,primary,fallback" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestRouterConfig(t *testing.T) {
	if err := router.RouteModel(""); err == nil || !strings.Contains(err.Error(), "router_config") {
		t.Fatalf("err = %v", err)
	}
}

func TestReviewModelEmpty(t *testing.T) {
	model, _ := router.ReviewModel("anthropic", []router.Family{{ID: "anthropic", Models: []string{}}})
	if model != "" {
		t.Fatalf("empty model = %s", model)
	}
}
