package rtree

import (
	"bytes"
	"log"
	"nautilus/internal/interpolate"
	"net/http"
	"slices"
	"strings"
)

const (
	MethodGet uint16 = 1 << iota
	MethodPost
	MethodPut
	MethodDelete
	MethodHead
	MethodConnect
	MethodOptions
	MethodTrace
	MethodPatch
	MethodAny uint16 = 0xFFF
)

// HTTPMethodMap maps standard HTTP method strings to internal bitmasks.
var HTTPMethodMap = map[string]uint16{
	http.MethodGet:     MethodGet,
	http.MethodPost:    MethodPost,
	http.MethodPut:     MethodPut,
	http.MethodDelete:  MethodDelete,
	http.MethodHead:    MethodHead,
	http.MethodConnect: MethodConnect,
	http.MethodOptions: MethodOptions,
	http.MethodTrace:   MethodTrace,
	http.MethodPatch:   MethodPatch,
}

// Edge represents the transition from a parent node to a child node.
type Edge struct {
	Fragment []byte     // Raw fragment used during tree construction
	Node     *RouteNode // Temporary pointer used during construction
	TargetID uint32     // Index of the destination node in NodePool (finalized)
	Offset   uint32     // Start position of the fragment in FragmentPool
	End      uint32     // End position of the fragment in FragmentPool
}

// RouteTree is the primary data structure for route indexing and searching.
type RouteTree struct {
	Root            [256]Edge // Entry points indexed by the first character
	FragmentPool    []byte    // Contiguous memory for all path fragments
	ActionsRegistry []string  // Registry of middleware & service identifiers
	ActionMetadata  []uint32
	NodePool        []RouteNode // Flattened node storage for cache locality
}

// RouteNode represents a specific point in the routing tree.
type RouteNode struct {
	Edges       []Edge // Outgoing transitions
	ActionIndex uint32
	Methods     uint16 // Bitmask of allowed HTTP methods; 0 if not a leaf
}

// backtrackState stores information for DFS-based wildcard searching.
type backtrackState struct {
	edge   *Edge
	urlIdx int
}

// Search looks up a URL in the tree and returns the matching RouteNode.
// Returns (node, true) if found, (nil, false) otherwise.
func (t *RouteTree) Search(url []byte) (*RouteNode, bool) {
	if len(url) == 0 {
		return nil, false
	}

	// Pre-allocate stack for backtracking to handle wildcards '*'
	stack := make([]backtrackState, 0, 8)
	urlIdx := 0
	urlLen := len(url)

	firstChar := url[0]
	var currentEdge *Edge

	// Initial root selection
	if t.Root[firstChar].TargetID != 0 {
		currentEdge = &t.Root[firstChar]
		if firstChar != '*' && t.Root['*'].TargetID != 0 {
			stack = append(stack, backtrackState{edge: &t.Root['*'], urlIdx: 0})
		}
	} else if t.Root['*'].TargetID != 0 {
		currentEdge = &t.Root['*']
	} else {
		return nil, false
	}

	for {
		node := &t.NodePool[currentEdge.TargetID]
		fStart, fEnd := currentEdge.Offset, currentEdge.End
		fLen := int(fEnd - fStart)

		// Handle Wildcard Fragment
		if t.FragmentPool[fStart] == '*' {
			if len(node.Edges) == 0 {
				if node.Methods != 0 {
					return node, true
				}
			} else {
				// Peek next expected static fragment after wildcard
				nextEdge := &node.Edges[0]
				targetFrag := t.FragmentPool[nextEdge.Offset:nextEdge.End]

				foundIdx := bytes.Index(url[urlIdx:], targetFrag)
				if foundIdx >= 0 {
					urlIdx += foundIdx
					currentEdge = nextEdge
					continue
				}
			}
			goto ATTEMPT_BACKTRACK
		}

		// Handle Static Fragment matching
		if urlIdx+fLen <= urlLen && bytes.Equal(url[urlIdx:urlIdx+fLen], t.FragmentPool[fStart:fEnd]) {
			urlIdx += fLen

			if urlIdx == urlLen {
				if node.Methods != 0 {
					return node, true
				}
				goto ATTEMPT_BACKTRACK
			}

			nextChar := url[urlIdx]
			var exactMatch *Edge
			var wildcardMatch *Edge

			for i := range node.Edges {
				e := &node.Edges[i]
				switch t.FragmentPool[e.Offset] {
				case nextChar:
					exactMatch = e
				case '*':
					wildcardMatch = e
				}
			}

			if exactMatch != nil {
				if wildcardMatch != nil {
					stack = append(stack, backtrackState{edge: wildcardMatch, urlIdx: urlIdx})
				}
				currentEdge = exactMatch
				continue
			} else if wildcardMatch != nil {
				currentEdge = wildcardMatch
				continue
			}
		}

	ATTEMPT_BACKTRACK:
		if len(stack) > 0 {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			currentEdge = last.edge
			urlIdx = last.urlIdx
			continue
		}
		break
	}

	return nil, false
}

// RawNode represents the input format for building a RouteTree.
type RawNode struct {
	URL         string
	Service     string
	Middlewares []string
	Methods     string // Comma-separated methods, e.g., "GET,POST"
}

// Build constructs a finalized RouteTree from a slice of RawNodes.
// Logs an error and returns nil if the input is empty.
func Build(rawNodes []*RawNode) *RouteTree {
	if len(rawNodes) == 0 {
		log.Println("[rtree] Build failed: no raw nodes provided")
		return nil
	}

	t := &RouteTree{
		FragmentPool: make([]byte, 0),
		NodePool:     make([]RouteNode, 0, len(rawNodes)),
	}

	actionMap := make(map[string]uint32)

	for _, raw := range rawNodes {
		url := ReverseHost(raw.URL)
		methodMask := parseMethods(raw.Methods)

		svcID := t.getOrCreateActionID(raw.Service, actionMap)

		mwIDs := make([]uint32, len(raw.Middlewares))
		for i, mw := range raw.Middlewares {
			mwIDs[i] = t.getOrCreateActionID(mw, actionMap)
		}
		actionIndex := uint32(len(t.ActionMetadata))

		mwCount := len(mwIDs)

		actions := make([]uint32, 2, mwCount+2)
		actions[0] = svcID
		actions[1] = uint32(mwCount)
		actions = append(actions, mwIDs...)

		t.ActionMetadata = append(t.ActionMetadata, actions...)

		t.insert(url, actionIndex, methodMask)
	}

	totalLen := t.compress()
	t.finalize(totalLen)

	log.Printf("[rtree] Successfully built tree with %d nodes", len(t.NodePool))
	return t
}

func (t *RouteTree) getOrCreateActionID(action string, actionMap map[string]uint32) uint32 {
	actionID, exists := actionMap[action]
	if !exists {
		id := uint32(len(t.ActionsRegistry))
		t.ActionsRegistry = append(t.ActionsRegistry, action)

		actionID = uint32(len(t.ActionMetadata))

		ops := interpolate.Analyze(action)

		l := len(ops)
		actionMetadata := make([]uint32, 2, l+2)
		actionMetadata[0] = id
		actionMetadata[1] = uint32(l)
		actionMetadata = append(actionMetadata, ops...)
		t.ActionMetadata = append(t.ActionMetadata, actionMetadata...)

		actionMap[action] = actionID
	}
	return actionID
}

func (t *RouteTree) insert(url []byte, actionIndex uint32, methods uint16) {
	if len(url) == 0 {
		return
	}

	firstChar := url[0]
	edge := &t.Root[firstChar]
	if edge.Node == nil {
		edge.Fragment = []byte{firstChar}
		edge.Node = &RouteNode{}
	}

	currNode := edge.Node
	for i := 1; i < len(url); i++ {
		char := url[i]
		var nextEdge *Edge
		for j := range currNode.Edges {
			if currNode.Edges[j].Fragment[0] == char {
				nextEdge = &currNode.Edges[j]
				break
			}
		}

		if nextEdge == nil {
			currNode.Edges = append(currNode.Edges, Edge{
				Node:     &RouteNode{},
				Fragment: []byte{char},
			})
			nextEdge = &currNode.Edges[len(currNode.Edges)-1]
		}
		currNode = nextEdge.Node
	}

	currNode.ActionIndex = actionIndex
	currNode.Methods = methods
}

// compress merges single-child nodes to form a radix tree.
func (t *RouteTree) compress() int {
	totalLen := 0
	for i := range 256 {
		if t.Root[i].Node != nil {
			totalLen += t.compressNode(t.Root[i].Node)
		}
	}
	return totalLen
}

func (t *RouteTree) compressEdge(e *Edge) (*Edge, int, bool) {
	if e.Node == nil {
		return nil, 0, false
	}

	switch len(e.Node.Edges) {
	case 0:
		if e.Fragment[0] != '*' {
			return e, 1, true
		}
	case 1:
		child, l, ok := t.compressEdge(&e.Node.Edges[0])
		if ok && e.Fragment[0] != '*' && e.Node.Methods == 0 {
			e.Fragment = append(e.Fragment, child.Fragment...)
			e.Node = child.Node
			return e, (l + 1), true
		} else {
			return nil, (l + 1), false
		}
	default:
		l := t.compressNode(e.Node)
		return nil, (l + 1), false
	}
	return nil, 1, false
}

func (t *RouteTree) compressNode(n *RouteNode) int {
	total := 0
	wildcardIdx := -1
	for i := range n.Edges {
		_, l, _ := t.compressEdge(&n.Edges[i])
		total += l
		if n.Edges[i].Fragment[0] == '*' {
			wildcardIdx = i
		}
	}
	// Ensure wildcard is always the last edge for searching priority
	if wildcardIdx > -1 {
		last := len(n.Edges) - 1
		n.Edges[wildcardIdx], n.Edges[last] = n.Edges[last], n.Edges[wildcardIdx]
	}
	return total
}

func (t *RouteTree) finalize(estimatedLen int) {
	t.NodePool = make([]RouteNode, 1) // 0 index is reserved/null
	t.FragmentPool = make([]byte, 0, estimatedLen)

	for i := range 256 {
		if t.Root[i].Node != nil {
			t.Root[i] = t.rebuildPool(&t.Root[i])
		}
	}
}

func (t *RouteTree) rebuildPool(e *Edge) Edge {
	edges := make([]Edge, len(e.Node.Edges))
	for i := range edges {
		edges[i] = t.rebuildPool(&e.Node.Edges[i])
	}

	offset := uint32(len(t.FragmentPool))
	t.FragmentPool = append(t.FragmentPool, e.Fragment...)
	end := uint32(len(t.FragmentPool))

	nodeID := uint32(len(t.NodePool))
	t.NodePool = append(t.NodePool, RouteNode{
		Edges:       edges,
		ActionIndex: e.Node.ActionIndex,
		Methods:     e.Node.Methods,
	})

	return Edge{
		TargetID: nodeID,
		Offset:   offset,
		End:      end,
	}
}

func parseMethods(methods string) uint16 {
	var res uint16
	for m := range strings.SplitSeq(strings.ToLower(methods), ",") {
		res |= matchMethodToken(strings.TrimSpace(m))
	}
	return res
}

func matchMethodToken(m string) uint16 {
	switch m {
	case "g", "get":
		return MethodGet
	case "p", "po", "post":
		return MethodPost
	case "pu", "put":
		return MethodPut
	case "d", "del", "delete":
		return MethodDelete
	case "head":
		return MethodHead
	case "connect":
		return MethodConnect
	case "options":
		return MethodOptions
	case "trace":
		return MethodTrace
	case "patch":
		return MethodPatch
	case "*", "any":
		return MethodAny
	default:
		if m != "" {
			log.Printf("[rtree] Warning: unknown HTTP method token ignored: %s", m)
		}
		return 0
	}
}

// ReverseHost reverses the host part of the URL for better indexing (e.g., com.google.www)
func ReverseHost(rawURL string) []byte {
	url := []byte(rawURL)
	slashIdx := slices.Index(url, '/')
	if slashIdx == -1 {
		slashIdx = len(url)
	}
	if slashIdx <= 1 {
		return url
	}

	hostPart := string(url[:slashIdx])
	segments := strings.Split(hostPart, ".")
	slices.Reverse(segments)

	newHost := strings.Join(segments, ".")
	return append([]byte(newHost), url[slashIdx:]...)
}
