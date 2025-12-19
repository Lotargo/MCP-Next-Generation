package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/Knetic/govaluate"
	"google.golang.org/protobuf/types/known/structpb"
	"strconv"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
)

// CalculatorTool implements the toolcore.Tool interface.
type CalculatorTool struct {
	pb.UnimplementedToolServer
}

const newSchema = `{
	"type": "object",
	"properties": {
		"expression": {
			"type": "string",
			"description": "A mathematical expression to evaluate. Example: '2 * (3 + 4)'"
		}
	},
	"required": ["expression"]
}`

// GetDescription returns the tool's description for gRPC.
func (t *CalculatorTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "calculator",
		Description:     "Basic math calculator. Args: expression (e.g. '2 + 2 * 5').",
		InputSchemaJson: newSchema,
	}, nil
}

// Run executes the calculator tool's logic for gRPC.
func (t *CalculatorTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	// processCalculation now ALWAYS returns a string.
	resultString := processCalculation(in.Arguments.AsMap())

	// CRITICAL FIX: The result must be packaged as a StringValue for protobuf.
	resultValue, err := structpb.NewValue(resultString)
	if err != nil {
		// This should realistically never fail for a simple string.
		return &pb.ToolRunResponse{Error: fmt.Sprintf("Error converting string result to protobuf value: %v", err)}, nil
	}
	return &pb.ToolRunResponse{Result: resultValue}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *CalculatorTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "calculator",
		Description: "Basic math calculator. Args: expression (e.g. '2 + 2 * 5').",
		InputSchema: newSchema,
	}
}

// StdioHandler handles the logic for the stdio transport.
func (t *CalculatorTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	resultString := processCalculation(arguments)
	return map[string]interface{}{"result": resultString}, nil
}

// processCalculation contains the unified logic for both transports.
// It always returns a string, either the formatted result or an error message.
func processCalculation(args map[string]interface{}) string {
	exprStr, ok := args["expression"].(string)
	if !ok || exprStr == "" {
		return "Error: Invalid or missing 'expression' argument."
	}

	expression, err := govaluate.NewEvaluableExpression(exprStr)
	if err != nil {
		return fmt.Sprintf("Error: Failed to parse expression. Details: %v", err)
	}

	parameters := make(map[string]interface{})
	result, err := expression.Evaluate(parameters)
	if err != nil {
		return fmt.Sprintf("Error: Failed to evaluate expression. Details: %v", err)
	}

	// Format the float64 result into a clean string.
	if f, ok := result.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}

	// Fallback for any other type, though govaluate primarily returns float64 for numerics.
	return fmt.Sprintf("%v", result)
}


func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	toolcore.Start(&CalculatorTool{}, *transport, *port)
}
