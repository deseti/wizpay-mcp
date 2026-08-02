package tools

import (
	"fmt"
	"sort"

	"github.com/deseti/wizpay-mcp/internal/services"
)

type Registry struct{ definitions map[string]Definition }

func NewRegistry(definitions ...Definition) (*Registry, error) {
	r := &Registry{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := r.Register(definition); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(definition Definition) error {
	if r == nil {
		return fmt.Errorf("MCP tool registry is required")
	}
	if err := definition.validate(); err != nil {
		return err
	}
	if _, exists := r.definitions[definition.Name()]; exists {
		return fmt.Errorf("duplicate MCP tool %q", definition.Name())
	}
	r.definitions[definition.Name()] = definition
	return nil
}

func (r *Registry) Lookup(name string) (Definition, bool) {
	definition, ok := r.definitions[name]
	return definition, ok
}
func (r *Registry) Definitions() []Definition {
	definitions := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name() < definitions[j].Name() })
	return definitions
}
func (r *Registry) Tools() []Tool {
	definitions := r.Definitions()
	result := make([]Tool, len(definitions))
	for index := range definitions {
		result[index] = definitions[index]
	}
	return result
}

func NewFoundationRegistry(bundle services.Bundle) (*Registry, error) {
	if bundle.Intents == nil || bundle.Approvals == nil || bundle.Policies == nil || bundle.Executions == nil {
		return nil, fmt.Errorf("all MCP domain services are required")
	}
	definitions, err := foundationDefinitions(bundle)
	if err != nil {
		return nil, err
	}
	return NewRegistry(definitions...)
}
