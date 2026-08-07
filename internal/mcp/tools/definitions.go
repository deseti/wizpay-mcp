package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	CreateIntentName     = "wizpay.create_intent"
	GetIntentName        = "wizpay.get_intent"
	RequestApprovalName  = "wizpay.request_approval"
	GetApprovalName      = "wizpay.get_approval"
	EvaluatePolicyName   = "wizpay.evaluate_policy"
	PrepareExecutionName = "wizpay.prepare_execution"
	ListSchedulesName    = "wizpay.autonomy.list_schedules"
	GetScheduleName      = "wizpay.autonomy.get_schedule"
	SimulateScheduleName = "wizpay.autonomy.simulate"
	CreateScheduleName   = "wizpay.autonomy.create_schedule"
	ControlScheduleName  = "wizpay.autonomy.control_schedule"
	EmergencyStopName    = "wizpay.autonomy.emergency_stop"
)

type Definition struct {
	name         string
	description  string
	inputSchema  *jsonschema.Schema
	outputSchema *jsonschema.Schema
	resolvedIn   *jsonschema.Resolved
	resolvedOut  *jsonschema.Resolved
	register     func(*sdkmcp.Server)
}

func (d Definition) Name() string                     { return d.name }
func (d Definition) Description() string              { return d.description }
func (d Definition) InputSchema() *jsonschema.Schema  { return d.inputSchema }
func (d Definition) OutputSchema() *jsonschema.Schema { return d.outputSchema }
func (d Definition) ValidateInput(value any) error {
	if d.resolvedIn == nil {
		return fmt.Errorf("tool %q has no resolved input schema", d.name)
	}
	return d.resolvedIn.Validate(value)
}
func (d Definition) ValidateOutput(value any) error {
	if d.resolvedOut == nil {
		return fmt.Errorf("tool %q has no resolved output schema", d.name)
	}
	return d.resolvedOut.Validate(value)
}
func (d Definition) Register(server *sdkmcp.Server) error {
	if server == nil {
		return fmt.Errorf("MCP server is required")
	}
	if err := d.validate(); err != nil {
		return err
	}
	d.register(server)
	return nil
}

func (d Definition) validate() error {
	if d.name == "" || d.description == "" || d.inputSchema == nil || d.outputSchema == nil || d.resolvedIn == nil || d.resolvedOut == nil || d.register == nil {
		return fmt.Errorf("incomplete MCP tool definition %q", d.name)
	}
	return nil
}

func newDefinition[In, Out any](name, description string, annotations *sdkmcp.ToolAnnotations, handler sdkmcp.ToolHandlerFor[In, Out]) (Definition, error) {
	in, err := jsonschema.For[In](nil)
	if err != nil {
		return Definition{}, fmt.Errorf("infer input schema for %s: %w", name, err)
	}
	resolvedIn, err := in.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		return Definition{}, fmt.Errorf("resolve input schema for %s: %w", name, err)
	}
	out, err := jsonschema.For[Out](nil)
	if err != nil {
		return Definition{}, fmt.Errorf("infer output schema for %s: %w", name, err)
	}
	resolvedOut, err := out.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		return Definition{}, fmt.Errorf("resolve output schema for %s: %w", name, err)
	}
	spec := &sdkmcp.Tool{Name: name, Description: description, InputSchema: in, OutputSchema: out, Annotations: annotations}
	return Definition{name: name, description: description, inputSchema: in, outputSchema: out, resolvedIn: resolvedIn, resolvedOut: resolvedOut,
		register: func(server *sdkmcp.Server) { sdkmcp.AddTool(server, spec, handler) }}, nil
}

func annotation(readOnly bool) *sdkmcp.ToolAnnotations {
	destructive, idempotent, openWorld := false, true, false
	return &sdkmcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: idempotent, OpenWorldHint: &openWorld}
}

func errorResult() *sdkmcp.CallToolResult { return &sdkmcp.CallToolResult{IsError: true} }

var _ Tool = Definition{}
