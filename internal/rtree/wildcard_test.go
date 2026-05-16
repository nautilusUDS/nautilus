package rtree_test

import (
	"nautilus/internal/rtree"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullWildcardHost(t *testing.T) {
	rawNodes := []*rtree.RawNode{
		{
			URL:     "*/api/test",
			Service: "all-hosts-service",
			Methods: "GET",
		},
	}

	tree := rtree.Build(rawNodes)
	require.NotNil(t, tree)

	url := []byte("example.com/api/test")
	urlBytes := rtree.ReverseHost(url)
	node, exists := tree.Search(urlBytes)

	assert.True(t, exists, "Expected match for full wildcard host '*'")
	if exists {
		serviceIndex := tree.ActionMetadata[node.ActionIndex]
		serviceID := tree.ActionMetadata[serviceIndex]
		assert.Equal(t, "all-hosts-service", tree.GetActionName(serviceID))
	}

	url2 := []byte("other.io/api/test")
	urlBytes2 := rtree.ReverseHost(url2)
	node2, exists2 := tree.Search(urlBytes2)

	assert.True(t, exists2, "Expected match for full wildcard host '*' on another domain")
	if exists2 {
		serviceIndex := tree.ActionMetadata[node2.ActionIndex]
		serviceID := tree.ActionMetadata[serviceIndex]
		assert.Equal(t, "all-hosts-service", tree.GetActionName(serviceID))
	}
}
