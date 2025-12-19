# Web Search Simple (Serper)

## Description

The `web_search_simple` tool is a lightweight Go-based tool for performing web searches using the **Serper.dev** API (Google Search wrapper). It is optimized for speed and simple factual queries, making it suitable for smaller or faster LLMs.

## Parameters

The tool accepts the following arguments in a JSON object:

| Parameter | Type     | Required | Description                     |
|-----------|----------|----------|---------------------------------|
| `query`   | `string` | **Yes**  | The search query.               |

## Usage Example

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "web_search_simple",
    "arguments": {
      "query": "current weather in Tokyo"
    }
  },
  "id": 1
}
```

## Configuration

The tool requires the `SERPER_API_KEY` secret.

**Configuration (`config.json`):**
```json
{
    "name": "web_search_simple",
    "description": "Simple web search using Serper.dev. Args: query.",
    "command": ["web_search_simple"],
    "required_secrets": ["SERPER_API_KEY"]
}
```
