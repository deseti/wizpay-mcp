package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/deseti/wizpay-mcp/internal/autonomy"
	"github.com/deseti/wizpay-mcp/internal/services"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func autonomyDefinitions(service services.AutonomyService) ([]Definition, error) {
	if service == nil {
		return nil, fmt.Errorf("autonomy service is required")
	}
	defs := make([]Definition, 0, 6)
	add := func(d Definition, err error) error {
		if err != nil {
			return err
		}
		defs = append(defs, d)
		return nil
	}
	d, e := newDefinition(ListSchedulesName, "List authenticated user's bounded autonomous schedules.", annotation(true), listSchedulesHandler(service))
	if err := add(d, e); err != nil {
		return nil, err
	}
	d, e = newDefinition(GetScheduleName, "Read one authenticated user's autonomous schedule metadata.", annotation(true), getScheduleHandler(service))
	if err := add(d, e); err != nil {
		return nil, err
	}
	d, e = newDefinition(SimulateScheduleName, "Simulate an autonomous occurrence without financial mutation or provider calls.", annotation(true), simulateHandler(service))
	if err := add(d, e); err != nil {
		return nil, err
	}
	d, e = newDefinition(CreateScheduleName, "Create one typed, bounded autonomous schedule.", annotation(false), createScheduleHandler(service))
	if err := add(d, e); err != nil {
		return nil, err
	}
	d, e = newDefinition(ControlScheduleName, "Pause, resume, or revoke a schedule by creating a new version.", annotation(false), controlScheduleHandler(service))
	if err := add(d, e); err != nil {
		return nil, err
	}
	d, e = newDefinition(EmergencyStopName, "Activate or deactivate the authenticated tenant's autonomous emergency stop.", annotation(false), emergencyStopHandler(service))
	if err := add(d, e); err != nil {
		return nil, err
	}
	return defs, nil
}
func listSchedulesHandler(s services.AutonomyService) sdkmcp.ToolHandlerFor[ListSchedulesInput, ListSchedulesResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListSchedulesInput) (*sdkmcp.CallToolResult, ListSchedulesResponse, error) {
		values, err := s.ListSchedules(ctx)
		if err != nil {
			return errorResult(), ListSchedulesResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		out := make([]ScheduleOutput, len(values))
		for i, v := range values {
			out[i] = scheduleOutput(v)
		}
		return nil, ListSchedulesResponse{Result: out}, nil
	}
}
func getScheduleHandler(s services.AutonomyService) sdkmcp.ToolHandlerFor[GetScheduleInput, ScheduleResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetScheduleInput) (*sdkmcp.CallToolResult, ScheduleResponse, error) {
		v, err := s.GetSchedule(ctx, in.ScheduleID, in.Version)
		if err != nil {
			return errorResult(), ScheduleResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		o := scheduleOutput(v)
		return nil, ScheduleResponse{Result: &o}, nil
	}
}
func simulateHandler(s services.AutonomyService) sdkmcp.ToolHandlerFor[SimulateScheduleInput, SimulateScheduleResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SimulateScheduleInput) (*sdkmcp.CallToolResult, SimulateScheduleResponse, error) {
		v, err := s.SimulateOccurrence(ctx, in.ScheduleID, in.Version, in.At)
		if err != nil {
			return errorResult(), SimulateScheduleResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		o := DecisionOutput{Eligible: v.Eligible, RequiresStepUp: v.RequiresStepUp, Reason: string(v.Reason), ScheduleID: v.ScheduleID, OccurrenceID: v.OccurrenceID, GrantID: v.GrantID}
		return nil, SimulateScheduleResponse{Result: &o}, nil
	}
}
func createScheduleHandler(s services.AutonomyService) sdkmcp.ToolHandlerFor[CreateScheduleInput, ScheduleResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in CreateScheduleInput) (*sdkmcp.CallToolResult, ScheduleResponse, error) {
		v, err := s.CreateSchedule(ctx, in.Schedule)
		if err != nil {
			return errorResult(), ScheduleResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		o := scheduleOutput(v)
		return nil, ScheduleResponse{Result: &o}, nil
	}
}
func controlScheduleHandler(s services.AutonomyService) sdkmcp.ToolHandlerFor[ControlScheduleInput, ScheduleResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ControlScheduleInput) (*sdkmcp.CallToolResult, ScheduleResponse, error) {
		v, err := s.SetScheduleStatus(ctx, in.ScheduleID, in.Version, in.Status)
		if err != nil {
			return errorResult(), ScheduleResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		o := scheduleOutput(v)
		return nil, ScheduleResponse{Result: &o}, nil
	}
}
func emergencyStopHandler(s services.AutonomyService) sdkmcp.ToolHandlerFor[EmergencyStopInput, EmergencyStopResponse] {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest, in EmergencyStopInput) (*sdkmcp.CallToolResult, EmergencyStopResponse, error) {
		v, err := s.SetEmergencyStop(ctx, autonomy.EmergencyStop{Active: in.Active, Scope: in.Scope, Reason: in.Reason})
		if err != nil {
			return errorResult(), EmergencyStopResponse{Error: publicToolError(in.RequestID, err)}, nil
		}
		o := EmergencyStopOutput{Active: v.Active, Scope: v.Scope, Reason: v.Reason, ChangedAt: v.ChangedAt.UTC().Format(time.RFC3339Nano)}
		return nil, EmergencyStopResponse{Result: &o}, nil
	}
}
func scheduleOutput(v autonomy.Schedule) ScheduleOutput {
	return ScheduleOutput{ScheduleID: v.ID, Version: v.Version, Digest: v.Digest, Status: string(v.Status), IntentType: string(v.Spec.Intent), WalletBindingID: v.WalletBindingID, GrantID: v.GrantID, CreatedAt: v.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: v.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}
