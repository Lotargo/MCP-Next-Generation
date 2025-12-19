# Web Search Tavily

## Description

The `web_search_tavily` tool is a Go-based gRPC service that performs a deep web search using the **Tavily AI search engine**. It is designed to provide up-to-date information, facts, and news by returning a list of relevant search results. It is generally more capable than the "simple" search but may be slower.

## Parameters

The tool accepts the following arguments in a JSON object:

| Parameter     | Type     | Required | Default | Description                                            |
|---------------|----------|----------|---------|--------------------------------------------------------|
| `query`       | `string` | **Yes**  |         | The search query or question.                          |
| `max_results` | `number` | No       | `5`     | The maximum number of search results to return.        |

## Response

*   **Successful Search:** If the search is successful, the tool returns a JSON array of objects, where each object represents a search result from the Tavily API. Each result typically contains a `title`, `url`, `content` (snippet), and `score`.
*   **Error:** If the Tavily API key is not configured, the query is missing, or the API returns an error, the tool returns an error message.

## Usage Example

Here is an example of how to use the `web_search` tool to find information about the Go programming language.

**Request:**

```bash
curl -X POST http://localhost:8080/ \
-H "Content-Type: application/json" \
-d '{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "web_search_tavily",
    "arguments": {
      "query": "What is the Go programming language?",
      "max_results": 2
    }
  },
  "id": 1
}'
```

**Successful Response:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "[{\"title\": \"The Go Programming Language\", \"url\": \"https://go.dev/\", ...}]"
      }
    ]
  }
}
```

## Configuration

The tool requires the `TAVILY_API_KEY` secret.

**Example `config.json`:**
```json
{
    "name": "web_search_tavily",
    "description": "Web search using Tavily API. Args: query, max_results.",
    "command": ["web_search_tavily"],
    "required_secrets": ["TAVILY_API_KEY"]
}
```

The server automatically injects the `TAVILY_API_KEY` via a temporary file path passed to the `--tavily_api_key-from-file` flag during startup.

## Health and Logging

*   **Health Checks:** Implements the standard gRPC Health Checking Protocol.
*   **Logging:** Uses a structured JSON logger (`slog`) for observability.

## Testing

To test the tool, you can run the provided integration tests. Navigate to the tool's directory and run:
```bash
go test ./...
```