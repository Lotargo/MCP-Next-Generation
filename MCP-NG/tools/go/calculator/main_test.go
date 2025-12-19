package main

import (
	"context"
	"strings"
	"testing"
	"google.golang.org/protobuf/types/known/structpb"
	pb "mcp/shared/proto"
)

// TestProcessCalculation tests the core logic of the calculator with its new string-based output.
func TestProcessCalculation(t *testing.T) {
	testCases := []struct {
		name              string
		expression        string
		expectedResult    string
		expectErrorString bool // True if the expectedResult is an error message substring
	}{
		{"Simple Addition", "2 + 2", "4", false},
		{"Floating Point Result", "27 / 21", "1.2857142857142858", false},
		{"User's Complex Expression (Corrected)", "2 + 222 / 24 * (234 * 42 / 12)", "7577.75", false},
		{"Division by Zero", "10 / 0", "+Inf", false},
		// Test for the user's specific syntax error
		{"Missing Multiplication Operator", "2+222/24(234*42/12)", "Error: Failed to parse expression", true},
		// General error cases
		{"Invalid Token", "5 +* 3", "Error: Failed to parse expression", true},
		{"Empty Expression", "", "Error: Invalid or missing 'expression' argument", true},
		{"Attempted Function Call", "os.Exit(1)", "Error: Failed to parse expression", true},
		{"Attempted Variable Access", "someVar", "Error: Failed to evaluate expression", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]interface{}{"expression": tc.expression}
			resultString := processCalculation(args)

			if tc.expectErrorString {
				if !strings.Contains(resultString, tc.expectedResult) {
					t.Errorf("Expected result to contain error '%s', but got '%s'", tc.expectedResult, resultString)
				}
			} else {
				if resultString != tc.expectedResult {
					t.Errorf("Expected result '%s', but got '%s'", tc.expectedResult, resultString)
				}
			}
		})
	}
}

// TestRunGRPC tests the gRPC server part of the tool with the new string-based output.
func TestRunGRPC(t *testing.T) {
	tool := &CalculatorTool{}
	args, _ := structpb.NewStruct(map[string]interface{}{"expression": "100 / 8"})
	req := &pb.ToolRunRequest{Arguments: args}

	res, err := tool.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("gRPC Run failed: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("gRPC Run returned an error: %s", res.Error)
	}

	resultVal, ok := res.Result.GetKind().(*structpb.Value_StringValue)
	if !ok {
		t.Fatalf("Expected a string value in result, but got something else")
	}

	if resultVal.StringValue != "12.5" {
		t.Errorf("Expected result '12.5', got '%s'", resultVal.StringValue)
	}
}
