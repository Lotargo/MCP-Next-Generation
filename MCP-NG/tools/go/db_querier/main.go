package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/types/known/structpb"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
	pb "mcp/shared/proto"
)

// DBQuerierTool implements the toolcore.Tool interface.
type DBQuerierTool struct {
	pb.UnimplementedToolServer
}

// GetDescription returns the tool's description.
func (t *DBQuerierTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "db_querier",
		Description:     "Run SQL queries on SQLite. Args: db_path, query. Warning: Can modify data.",
		InputSchemaJson: `{"type":"object","properties":{"db_path":{"type":"string","description":"Path to SQLite DB."},"query":{"type":"string","description":"SQL query."}},"required":["db_path","query"]}`,
	}, nil
}

// Run executes the db_querier tool's logic for gRPC.
func (t *DBQuerierTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	args := in.Arguments.AsMap()
	results, err := t.executeQuery(ctx, args)
	if err != nil {
		return &pb.ToolRunResponse{Error: err.Error()}, nil
	}
	resultValue, err := structpb.NewValue(results)
	if err != nil {
		return &pb.ToolRunResponse{Error: fmt.Sprintf("Error creating result value: %v", err)}, nil
	}
	return &pb.ToolRunResponse{Result: resultValue}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *DBQuerierTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "db_querier",
		Description: "Run SQL queries on SQLite. Args: db_path, query. Warning: Can modify data.",
		InputSchema: `{"type":"object","properties":{"db_path":{"type":"string"},"query":{"type":"string"}},"required":["db_path","query"]}`,
	}
}

// StdioHandler handles the logic for the stdio transport.
func (t *DBQuerierTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	results, err := t.executeQuery(context.Background(), arguments)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"result": results}, nil
}

// executeQuery is the shared business logic.
func (t *DBQuerierTool) executeQuery(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	dbPath, ok := args["db_path"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid or missing 'db_path' argument")
	}
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid or missing 'query' argument")
	}

	absPath := filepath.Clean(dbPath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file not found: '%s'", dbPath)
	}

	db, err := sql.Open("sqlite3", absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("SQL query error: %v", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %v", err)
	}

	var results []interface{}
	for rows.Next() {
		rowValues := make([]interface{}, len(columns))
		rowPointers := make([]interface{}, len(columns))
		for i := range rowValues {
			rowPointers[i] = &rowValues[i]
		}

		if err := rows.Scan(rowPointers...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			val := rowValues[i]
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	toolcore.Start(&DBQuerierTool{}, *transport, *port)
}
