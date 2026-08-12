/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestParseWatchNamespaces(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "whitespace only", in: "   ", want: nil},
		{name: "single", in: "team-a", want: []string{"team-a"}},
		{name: "multiple", in: "team-a,team-b,team-c", want: []string{"team-a", "team-b", "team-c"}},
		{name: "trims spaces", in: " team-a , team-b ", want: []string{"team-a", "team-b"}},
		{name: "drops empty entries", in: "team-a,,team-b,", want: []string{"team-a", "team-b"}},
		{name: "deduplicates", in: "team-a,team-b,team-a", want: []string{"team-a", "team-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWatchNamespaces(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseWatchNamespaces(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Regression tests for #586: adding the operator namespace to DefaultNamespaces
// started informers there for every watched type, which the chart's namespaced
// Roles do not grant, so the manager failed to start.
func TestBuildCacheOptions_DefaultNamespacesExcludeOperatorNamespace(t *testing.T) {
	opts := buildCacheOptions([]string{"team-a", "team-b"}, "openclaw-operator-system")

	if _, ok := opts.DefaultNamespaces["openclaw-operator-system"]; ok {
		t.Error("operator namespace must not be in DefaultNamespaces - it would start an informer per watched type there")
	}
	for _, ns := range []string{"team-a", "team-b"} {
		if _, ok := opts.DefaultNamespaces[ns]; !ok {
			t.Errorf("watched namespace %q missing from DefaultNamespaces", ns)
		}
	}
	if len(opts.DefaultNamespaces) != 2 {
		t.Errorf("DefaultNamespaces = %d entries, want 2", len(opts.DefaultNamespaces))
	}
}

func TestBuildCacheOptions_SecretsReachOperatorNamespace(t *testing.T) {
	opts := buildCacheOptions([]string{"team-a"}, "openclaw-operator-system")

	var secretCfg *cache.ByObject
	for obj, cfg := range opts.ByObject {
		if _, ok := obj.(*corev1.Secret); ok {
			c := cfg
			secretCfg = &c
		}
	}
	if secretCfg == nil {
		t.Fatal("expected a per-object cache config for Secrets")
	}

	// The backup credentials Secret lives in the operator namespace, and the
	// watched namespaces still need their own Secret reads.
	for _, ns := range []string{"team-a", "openclaw-operator-system"} {
		if _, ok := secretCfg.Namespaces[ns]; !ok {
			t.Errorf("Secret informer should cover namespace %q", ns)
		}
	}
}

// When the operator watches its own namespace, it must not appear twice.
func TestBuildCacheOptions_OperatorNamespaceAlsoWatched(t *testing.T) {
	opts := buildCacheOptions([]string{"openclaw-operator-system", "team-a"}, "openclaw-operator-system")

	if len(opts.DefaultNamespaces) != 2 {
		t.Errorf("DefaultNamespaces = %d entries, want 2", len(opts.DefaultNamespaces))
	}
	if _, ok := opts.DefaultNamespaces["openclaw-operator-system"]; !ok {
		t.Error("an explicitly watched operator namespace should still be in DefaultNamespaces")
	}
}
