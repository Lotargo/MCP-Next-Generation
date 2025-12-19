module mcp/tool/calculator

go 1.24.4

require (
	github.com/Knetic/govaluate v3.0.0+incompatible
	google.golang.org/protobuf v1.36.10
	mcp/shared/goutils v0.0.0
	mcp/shared/proto v0.0.0
)

require (
	golang.org/x/net v0.46.1-0.20251013234738-63d1a5100f82 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
)

replace mcp/shared/goutils => ../../../../mcp-shared-goutils

replace mcp/shared/proto => ../../../../mcp-shared-proto
