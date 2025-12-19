import gradio as gr
import requests
import os
import json
import time
import google.generativeai as genai
from google.ai.generativelanguage import FunctionDeclaration, Tool

# Configuration
GATEKEEPER_URL = "http://127.0.0.1:8082"
PROJECT_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "../../"))
LOG_FILE = os.path.join(PROJECT_ROOT, "logs", "server.json")
TOKEN_FILE = os.path.join(PROJECT_ROOT, "env", "ADMIN_TOKEN")
GEMINI_KEY_FILE = os.path.join(PROJECT_ROOT, "env", "GEMINI_API_KEY")

class MCPClient:
    def __init__(self):
        self.token = self._load_token()
        self.session = requests.Session()
        self.session.headers.update({"X-MCP-Admin-Key": self.token})

    def _load_token(self):
        if os.path.exists(TOKEN_FILE):
            try:
                with open(TOKEN_FILE, 'r') as f:
                    return f.read().strip()
            except Exception as e:
                print(f"Error reading token file: {e}")
        return ""

    def get_config(self):
        try:
            resp = self.session.get(f"{GATEKEEPER_URL}/admin/config", timeout=2)
            if resp.status_code == 200:
                return resp.json()
            return {"error": f"Status {resp.status_code}: {resp.text}"}
        except Exception as e:
            return {"error": str(e)}

    def set_config(self, log_level, tracing):
        payload = {"log_level": log_level, "tracing_enabled": tracing}
        try:
            resp = self.session.post(f"{GATEKEEPER_URL}/admin/config", json=payload, timeout=2)
            if resp.status_code == 200:
                return f"Success: {resp.json()}"
            return f"Error {resp.status_code}: {resp.text}"
        except Exception as e:
            return f"Exception: {e}"

    def scan_tools(self):
        try:
            resp = self.session.post(f"{GATEKEEPER_URL}/admin/tools/scan", timeout=15)
            if resp.status_code == 200:
                return resp.text
            return f"Error {resp.status_code}: {resp.text}"
        except Exception as e:
            return str(e)

    def _get_session(self):
        try:
            init_payload = {"jsonrpc": "2.0", "method": "initialize", "params": {}, "id": 1}
            # Note: initialize might need a fresh session or different headers?
            # Standard POST to / should work.
            resp = self.session.post(f"{GATEKEEPER_URL}/", json=init_payload, timeout=2)
            return resp.headers.get("Mcp-Session-Id")
        except:
            return None

    def list_tools_full(self):
        sid = self._get_session()
        if not sid: return []

        headers = {"Mcp-Session-Id": sid}
        try:
            resp = self.session.post(f"{GATEKEEPER_URL}/", json={"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}, headers=headers, timeout=2)
            data = resp.json()
            if "result" in data and "tools" in data["result"]:
                return data["result"]["tools"]
        except:
            pass
        return []

    def list_tools(self):
        tools = self.list_tools_full()
        if not tools: return [["Error", "Failed to fetch tools"]]
        return [[t["name"], t["description"]] for t in tools]

    def call_tool(self, name, args):
        sid = self._get_session()
        if not sid: return "Error: No Session"

        headers = {
            "Mcp-Session-Id": sid,
            "X-Mcp-Sync": "true"
        }

        payload = {
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {"name": name, "arguments": args},
            "id": 3
        }

        try:
            resp = self.session.post(f"{GATEKEEPER_URL}/", json=payload, headers=headers, timeout=30)
            data = resp.json()
            if "result" in data:
                content = data["result"].get("content", [])
                if content and len(content) > 0:
                    return content[0].get("text", "")
                return str(data["result"])
            if "error" in data:
                return f"Tool Error: {data['error'].get('message')}"
            return str(data)
        except Exception as e:
            return f"Call failed: {e}"

    def get_logs(self, count=50, level=None):
        params = {"count": count}
        if level: params["level"] = level
        try:
            resp = self.session.get(f"{GATEKEEPER_URL}/admin/logs", params=params, timeout=5)
            if resp.status_code == 200:
                return resp.json()
            return [f"Error fetching logs: {resp.status_code} {resp.text}"]
        except Exception as e:
            return [f"Exception fetching logs: {e}"]

    def read_logs(self, lines=50, min_level=None):
        raw_lines = self.get_logs(count=lines, level=min_level)
        formatted = []
        for line in raw_lines:
             try:
                j = json.loads(line)
                ts = j.get("time", "")
                lvl = j.get("level", "INFO")
                msg = j.get("msg", "")
                extras = {k:v for k,v in j.items() if k not in ["time", "level", "msg"]}
                formatted.append(f"[{ts}] {lvl}: {msg} {extras}")
             except:
                formatted.append(str(line).strip())
        return "\n".join(formatted)

class GeminiAgent:
    def __init__(self, client):
        self.client = client
        self.api_key = self._load_key()
        self.chat = None
        if self.api_key:
            genai.configure(api_key=self.api_key)
            self.model_name = "models/gemini-flash-lite-latest"

    def _load_key(self):
        if os.path.exists(GEMINI_KEY_FILE):
            try:
                with open(GEMINI_KEY_FILE, 'r') as f:
                    return f.read().strip()
            except:
                pass
        return ""

    def _convert_schema(self, schema):
        if not isinstance(schema, dict): return schema
        new_schema = schema.copy()
        if "type" in new_schema:
            new_schema["type"] = new_schema["type"].upper()
        if "properties" in new_schema:
            new_props = {}
            for k, v in new_schema["properties"].items():
                new_props[k] = self._convert_schema(v)
            new_schema["properties"] = new_props
        if "items" in new_schema:
            new_schema["items"] = self._convert_schema(new_schema["items"])
        return new_schema

    def _get_gemini_tools(self):
        mcp_tools = self.client.list_tools_full()
        funcs = []
        for t in mcp_tools:
            schema = t.get("inputSchema", {})
            if "type" not in schema: schema["type"] = "object"
            gemini_schema = self._convert_schema(schema)
            fd = FunctionDeclaration(
                name=t["name"],
                description=t["description"],
                parameters=gemini_schema
            )
            funcs.append(fd)
        if not funcs: return None
        return Tool(function_declarations=funcs)

    def init_chat(self):
        if not self.api_key: return False
        try:
            tools = self._get_gemini_tools()
            self.model = genai.GenerativeModel(self.model_name, tools=[tools] if tools else None)
            self.chat = self.model.start_chat(enable_automatic_function_calling=False)
            return True
        except Exception as e:
            print(f"Chat init error: {e}")
            return False

    def send_message(self, message, history):
        if not self.api_key:
            return "Error: GEMINI_API_KEY not found in env/."
        if not self.chat or not history:
            if not self.init_chat():
                return "Failed to initialize chat (check logs)."
        try:
            response = self.chat.send_message(message)
            while True:
                if not response.candidates:
                    return "Error: No response candidates."
                if not response.candidates[0].content.parts:
                     return "Error: Empty response content."
                part = response.candidates[0].content.parts[0]
                if part.function_call:
                    fc = part.function_call
                    tool_name = fc.name
                    args = dict(fc.args)
                    print(f"Gemini calling tool: {tool_name} {args}")
                    result = self.client.call_tool(tool_name, args)
                    print(f"Tool result: {result}")
                    response = self.chat.send_message(
                        genai.protos.Content(
                            parts=[genai.protos.Part(
                                function_response=genai.protos.FunctionResponse(
                                    name=tool_name,
                                    response={"result": result}
                                )
                            )]
                        )
                    )
                else:
                    return response.text
        except Exception as e:
            return f"Error: {e}"

client = MCPClient()
agent = GeminiAgent(client)

def update_dashboard(filter_level="INFO"):
    config = client.get_config()
    tools = client.list_tools()

    server_log_level = config.get("log_level", "INFO") if "error" not in config else "INFO"
    tracing = config.get("tracing_enabled", False) if "error" not in config else False

    # Read logs based on user filter, NOT server config level
    logs = client.read_logs(min_level=filter_level)

    return tools, logs, server_log_level, tracing

with gr.Blocks(title="MCP-NG Admin") as demo:
    gr.Markdown("# MCP-NG Admin Panel")

    with gr.Tab("Monitor"):
        with gr.Row():
            refresh_all_btn = gr.Button("Refresh All", variant="primary")
            # Separate Filter Radio for VIEWING logs
            filter_radio = gr.Radio(["DEBUG", "INFO", "WARN", "ERROR"], label="Log Filter (View)", value="INFO")

        gr.Markdown("### Active Tools")
        tools_table = gr.Dataframe(headers=["Name", "Description"], datatype=["str", "str"], interactive=False)

        gr.Markdown("### Server Logs")
        logs_output = gr.Textbox(label="Last 50 Logs", lines=20, interactive=False)

    with gr.Tab("Chat"):
        gr.Markdown("### AI Assistant (Gemini)")
        chatbot = gr.ChatInterface(fn=agent.send_message, title="MCP-NG Agent Chat")

    with gr.Tab("Control"):
        gr.Markdown("### Server Configuration")
        with gr.Row():
            # Separate Radio for SETTING config
            config_log_level_radio = gr.Radio(["DEBUG", "INFO", "WARN", "ERROR"], label="Server Log Level (Write)")
            tracing_checkbox = gr.Checkbox(label="Enable Tracing (X-Trace-Id)")
        apply_config_btn = gr.Button("Apply Configuration")
        config_status = gr.Textbox(label="Status", interactive=False)

        gr.Markdown("### Actions")
        scan_btn = gr.Button("Hot Reload Tools (Scan)")
        scan_status = gr.Textbox(label="Scan Result", interactive=False)

    # Event wiring

    # Refresh updates everything: Tools, Logs (using filter), and Control Panel state
    refresh_all_btn.click(
        fn=update_dashboard,
        inputs=[filter_radio],
        outputs=[tools_table, logs_output, config_log_level_radio, tracing_checkbox]
    )

    # Load initial state
    demo.load(
        fn=update_dashboard,
        inputs=[filter_radio],
        outputs=[tools_table, logs_output, config_log_level_radio, tracing_checkbox]
    )

    # Dynamic Log Polling (only updates logs output)
    def update_logs_only(lvl):
        return client.read_logs(min_level=lvl)

    # Increased interval to 3s to reduce load
    gr.Timer(3).tick(fn=update_logs_only, inputs=[filter_radio], outputs=[logs_output])

    apply_config_btn.click(
        fn=client.set_config,
        inputs=[config_log_level_radio, tracing_checkbox],
        outputs=config_status
    )

    scan_btn.click(fn=client.scan_tools, outputs=scan_status)

if __name__ == "__main__":
    print(f"Starting Admin UI. Token loaded: {bool(client.token)}")
    demo.launch(server_name="0.0.0.0", server_port=7860)
