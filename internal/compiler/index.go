package compiler

import (
	"bufio"
	"fmt"
	"io"
	"nautilus/internal/core/builtins/builtinsmware"
	"nautilus/internal/core/builtins/virtualservices"
	"nautilus/internal/rtree"
	"strings"

	"github.com/google/shlex"
)

type RawRule struct {
	Methods     string
	URL         string
	Service     string
	Middlewares []string
}

func Parse(r io.Reader) (*rtree.RouteTree, error) {
	var rawRules []RawRule
	var currentRule *RawRule
	var skippingUntilBlank bool

	scanner := bufio.NewScanner(r)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			skippingUntilBlank = false
			continue
		}

		if skippingUntilBlank {
			continue
		}

		if strings.HasPrefix(trimmed, "#*") {
			skippingUntilBlank = true
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		isIndent := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")

		if !isIndent {
			fields, err := shlex.Split(trimmed)
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid rule syntax: %s", lineCount, trimmed)
			}

			rule := RawRule{}

			switch len(fields) {
			case 0:
				continue
			case 1:
				rule.Methods, rule.URL, rule.Service = "*", "*/[|*]", fields[0]
			case 2:
				rule.Methods, rule.URL, rule.Service = "*", fields[0], fields[1]
			case 3:
				rule.Methods, rule.URL, rule.Service = fields[0], fields[1], fields[2]
			default:
				return nil, fmt.Errorf("line %d: invalid rule fields (expected 1-3, got %d): %s", lineCount, len(fields), trimmed)
			}

			// Compile-time validation for virtual services
			if strings.HasPrefix(rule.Service, "$") {
				valid, name := virtualservices.IsValid(rule.Service)
				if !valid {
					if name == "" {
						return nil, fmt.Errorf("line %d: invalid virtual service syntax: %s", lineCount, rule.Service)
					}
					return nil, fmt.Errorf("line %d: unknown virtual service: %s", lineCount, name)
				}
			}

			rawRules = append(rawRules, rule)
			currentRule = &rawRules[len(rawRules)-1]

		} else {
			if currentRule == nil {
				fmt.Printf("warning: line %d: unexpected indent without a preceding rule, skipping: %q\n", lineCount, trimmed)
				continue
			}
			// Compile-time validation for built-in middlewares
			if strings.HasPrefix(trimmed, "$") {
				valid, name := builtinsmware.IsValid(trimmed)
				if !valid {
					if name == "" {
						return nil, fmt.Errorf("line %d: invalid builtin middleware syntax: %s", lineCount, trimmed)
					}
					return nil, fmt.Errorf("line %d: unknown builtin middleware: %s", lineCount, name)
				}
			}
			currentRule.Middlewares = append(currentRule.Middlewares, trimmed)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error at line %d: %w", lineCount, err)
	}

	var rawNodes []*rtree.RawNode
	for _, rule := range rawRules {
		for _, url := range expandField(rule.URL) {
			rawNodes = append(rawNodes, &rtree.RawNode{
				Methods:     rule.Methods,
				URL:         url,
				Service:     rule.Service,
				Middlewares: rule.Middlewares,
			})
		}
	}

	return rtree.Build(rawNodes), nil
}

func ParseString(content string) (*rtree.RouteTree, error) {
	return Parse(strings.NewReader(content))
}
