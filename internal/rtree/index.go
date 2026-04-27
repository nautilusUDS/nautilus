package rtree

import (
	"bytes"
	"net/http"
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
	MethodAny uint16 = 0xFFFF
)

var Methods = map[string]uint16{
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

type MiddlewarePool = map[uint32]http.HandlerFunc

type RouteTree struct {
	Root           []*RouteNode
	FragmentPool   []byte
	MiddlewarePool []string
}

type RouteNode struct {
	fragment    []byte
	Children    []*RouteNode
	Middlewares []uint32
	Service     string
	FragOffset  uint32
	FragEnd     uint32
	Offset      uint16
	End         uint16
	Methods     uint16
	IsWildcard  bool
	IsLeaf      bool
}

func (t *RouteTree) Search(url []byte) (*RouteNode, bool) {
	currNodes := t.Root
	urlIdx := 0
	l := len(url)

	for {
		found := false
		for _, node := range currNodes {
			if node.IsWildcard {
				if len(node.Children) > 0 {
					child := node.Children[0]
					targetFrag := t.FragmentPool[child.FragOffset:child.FragEnd]

					startSearch := urlIdx
					foundIdx := bytes.Index(url[startSearch:], targetFrag)

					if foundIdx > 0 {
						urlIdx = startSearch + foundIdx
						currNodes = node.Children
						found = true
						break
					}
				} else if node.IsLeaf && urlIdx < l {
					return node, true
				}
				continue
			}

			fStart, fEnd := node.FragOffset, node.FragEnd
			fLen := int(fEnd - fStart)
			if urlIdx+fLen <= l && bytes.Equal(url[urlIdx:urlIdx+fLen], t.FragmentPool[fStart:fEnd]) {
				urlIdx += fLen
				if node.IsLeaf && urlIdx == l {
					return node, true
				}
				currNodes = node.Children
				found = true
				break
			}
		}

		if !found {
			break
		}
	}
	return nil, false
}

type RawNode struct {
	URL         string
	Service     string
	Middlewares []string
	Methods     string
}

func Build(rawNodes []*RawNode) *RouteTree {

	t := &RouteTree{
		Root:         make([]*RouteNode, 0),
		FragmentPool: make([]byte, 0),
	}

	middlewaresMap := make(map[string]uint32)

	for _, node := range rawNodes {
		url := ReverseHost(node.URL)
		service := node.Service
		methods := getMethods(node.Methods)

		middlewares := make([]uint32, len(node.Middlewares))
		for i, mw := range node.Middlewares {
			id, exists := middlewaresMap[mw]
			if !exists {
				id = uint32(len(t.MiddlewarePool))
				middlewaresMap[mw] = id
				t.MiddlewarePool = append(t.MiddlewarePool, mw)
			}
			middlewares[i] = id
		}

		t.insert(url, service, middlewares, methods)
	}

	t.compressNodes(&t.Root)
	t.finalizePool(t.Root)

	return t
}

func (t *RouteTree) insert(url []byte, service string, middlewares []uint32, methods uint16) {
	if t.Root == nil {
		t.Root = make([]*RouteNode, 0)
	}

	currNodes := &t.Root

	l := len(url)
	for i := range l {
		char := url[i]
		isWildcard := (char == '*')

		var foundNode *RouteNode

		for _, node := range *currNodes {
			if isWildcard && node.IsWildcard {
				foundNode = node
				break
			}
			if !isWildcard && !node.IsWildcard && len(node.fragment) == 1 && node.fragment[0] == char {
				foundNode = node
				break
			}
		}

		if foundNode == nil {
			foundNode = &RouteNode{
				fragment:   []byte{char},
				IsWildcard: isWildcard,
				Offset:     uint16(i),
				End:        uint16(i + 1),
			}
			*currNodes = append(*currNodes, foundNode)
		}

		if i == l-1 {
			foundNode.IsLeaf = true
			foundNode.Service = service
			foundNode.Middlewares = middlewares
			foundNode.Methods = methods
		} else {
			currNodes = &foundNode.Children
		}
	}
}

func (t *RouteTree) compressNodes(nodes *[]*RouteNode) {
	for _, node := range *nodes {
		if len(node.Children) > 0 {
			t.compressNodes(&node.Children)
		}

		for len(node.Children) == 1 && !node.IsLeaf && !node.IsWildcard && !node.Children[0].IsWildcard {
			child := node.Children[0]

			isNodeHost := node.Offset > node.End
			isChildHost := child.Offset > child.End
			if isNodeHost != isChildHost {
				break
			}

			node.fragment = append(node.fragment, child.fragment...)
			node.End = child.End

			node.IsLeaf = child.IsLeaf
			node.Service = child.Service
			node.Methods = child.Methods
			node.Middlewares = child.Middlewares

			node.Children = child.Children
		}
	}
}

func getMethods(methods string) uint16 {
	var res uint16

	for method := range strings.SplitSeq(strings.ToLower(methods), ",") {
		res |= getMethod(method)
	}

	return res
}

func getMethod(m string) uint16 {
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
		return 0
	}
}

func (t *RouteTree) finalizePool(nodes []*RouteNode) {
	for _, node := range nodes {
		node.FragOffset = uint32(len(t.FragmentPool))
		t.FragmentPool = append(t.FragmentPool, node.fragment...)
		node.FragEnd = uint32(len(t.FragmentPool))

		node.fragment = nil

		if len(node.Children) > 0 {
			t.finalizePool(node.Children)
		}
	}
}

func ReverseHost(rawURL string) []byte {
	url := []byte(rawURL)
	slash := bytes.Index(url, []byte("/"))
	if slash == -1 {
		slash = len(url)
	}
	for i := range slash / 2 {
		url[i], url[slash-1-i] = url[slash-1-i], url[i]
	}
	return url
}
