package scalable

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fakeStrimziLister simulates a cluster which only serves the given Strimzi API versions.
func fakeStrimziLister(served map[string]bool, calls *[]string) func(context.Context, *unstructured.UnstructuredList) error {
	return func(_ context.Context, list *unstructured.UnstructuredList) error {
		gvk := list.GroupVersionKind()
		*calls = append(*calls, gvk.Version)

		if !served[gvk.Version] {
			return &apimeta.NoKindMatchError{GroupKind: gvk.GroupKind(), SearchedVersions: []string{gvk.Version}}
		}

		item := unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "connect"}}}
		list.Items = []unstructured.Unstructured{item}

		return nil
	}
}

func TestListStrimziResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		served       map[string]bool
		wantVersion  string
		wantCalls    []string
		wantNoMatch  bool
		wantItemsLen int
	}{
		{name: "cluster serves v1", served: map[string]bool{"v1": true}, wantVersion: "v1", wantCalls: []string{"v1"}, wantItemsLen: 1},
		{
			name: "cluster serves both, newest wins", served: map[string]bool{"v1": true, "v1beta2": true},
			wantVersion: "v1", wantCalls: []string{"v1"}, wantItemsLen: 1,
		},
		{
			name: "cluster serves only v1beta2", served: map[string]bool{"v1beta2": true},
			wantVersion: "v1beta2", wantCalls: []string{"v1", "v1beta2"}, wantItemsLen: 1,
		},
		{name: "cluster serves none", served: map[string]bool{}, wantCalls: []string{"v1", "v1beta2"}, wantNoMatch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls []string

			items, gvk, err := listStrimziResources(context.Background(), fakeStrimziLister(tt.served, &calls), kafkaConnectGVK)

			assert.Equal(t, tt.wantCalls, calls)

			if tt.wantNoMatch {
				require.Error(t, err)
				assert.True(t, apimeta.IsNoMatchError(err))

				return
			}

			require.NoError(t, err)
			assert.Len(t, items, tt.wantItemsLen)
			assert.Equal(t, tt.wantVersion, gvk.Version)
			assert.Equal(t, kafkaConnectGVK.Kind, gvk.Kind)
			assert.Equal(t, kafkaConnectGVK.Group, gvk.Group)
		})
	}
}

func TestListStrimziResources_OtherErrorsAreNotHidden(t *testing.T) {
	t.Parallel()

	boom := errors.New("api server unavailable")

	var tries int

	_, _, err := listStrimziResources(context.Background(), func(context.Context, *unstructured.UnstructuredList) error {
		tries++
		return boom
	}, kafkaConnectGVK)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, tries, "a non no-match error must stop the fallback")
}

func TestGVKOrDefault(t *testing.T) {
	t.Parallel()

	legacy := schema.GroupVersionKind{Group: kafkaStrimziGroup, Version: kafkaStrimziLegacyVersion, Kind: kafkaConnectKind}

	withGVK := &unstructured.Unstructured{}
	withGVK.SetGroupVersionKind(legacy)

	assert.Equal(t, legacy, gvkOrDefault(withGVK, kafkaConnectGVK))
	assert.Equal(t, kafkaConnectGVK, gvkOrDefault(&unstructured.Unstructured{}, kafkaConnectGVK))
	assert.Equal(t, kafkaConnectGVK, gvkOrDefault(nil, kafkaConnectGVK))
}
