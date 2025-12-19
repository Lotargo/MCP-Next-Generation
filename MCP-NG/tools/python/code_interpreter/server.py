import argparse
import sys
import json
import io
import os
import logging
import contextlib
from concurrent import futures
import grpc
from google.protobuf import struct_pb2

# It is assumed that the project is installed in editable mode,
# so that this import can be resolved correctly.
from mcp_shared_proto_py import mcp_pb2
from mcp_shared_proto_py import mcp_pb2_grpc

class JsonFormatter(logging.Formatter):
    def format(self, record):
        log_record = {
            "level": record.levelname,
            "message": record.getMessage(),
            "time": self.formatTime(record, self.datefmt),
            "name": record.name
        }
        if record.exc_info:
            log_record["exception"] = self.formatException(record.exc_info)
        return json.dumps(log_record)

def setup_logging(tool_name):
    # Determine log path relative to the tool execution directory (MCP-NG/tools/python/code_interpreter)
    # We want to go up to the root 'logs' directory.
    # Use __file__ to be independent of CWD.
    base_dir = os.path.dirname(os.path.abspath(__file__))
    log_dir = os.path.join(base_dir, "../../../../logs")
    log_dir = os.path.abspath(log_dir)
    os.makedirs(log_dir, exist_ok=True)
    log_file = os.path.join(log_dir, f"{tool_name}.json")

    logger = logging.getLogger()
    logger.setLevel(logging.INFO)

    # File handler (JSON)
    file_handler = logging.FileHandler(log_file)
    file_handler.setFormatter(JsonFormatter())
    logger.addHandler(file_handler)

    # Stderr handler (Text) - preserve existing behavior
    stderr_handler = logging.StreamHandler(sys.stderr)
    stderr_handler.setFormatter(logging.Formatter('%(asctime)s - %(name)s - %(levelname)s - %(message)s'))
    logger.addHandler(stderr_handler)

    logging.info(f"Logging initialized. Writing JSON to {log_file}")

class CodeInterpreterTool(mcp_pb2_grpc.ToolServicer):
    """
    Implements the gRPC service for the Code Interpreter tool.
    """
    def GetDescription(self, request, context):
        """
        Returns the description of the tool, including its name,
        a brief description of what it does, and the JSON schema
        for its input arguments.
        """
        schema = {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string",
                    "description": "The Python code to be executed. IMPORTANT: For Windows paths, ALWAYS use raw strings (r'C:\\path\\to\\file') or double escape backslashes ('C:\\\\path\\\\to\\\\file') to prevent syntax errors. For Unix/Linux paths, use standard forward slashes ('/path/to/file')."
                }
            },
            "required": ["code"]
        }
        return mcp_pb2.ToolDescription(
            name="code_interpreter",
            description="Run Python code (sandboxed). No file I/O allowed. Returns stdout.",
            input_schema_json=json.dumps(schema)
        )

    def Run(self, request, context):
        """
        Executes the tool with the provided arguments.
        """
        args = dict(request.arguments)
        code_to_run = args.get("code")

        logging.info(f"Run called. Code length: {len(code_to_run) if code_to_run else 0}")

        if not code_to_run:
            logging.error("Error: 'code' argument is missing or empty.")
            # Return an error message within the response, which is more helpful for an LLM
            # than a gRPC-level error.
            result_value = struct_pb2.Value()
            result_value.string_value = "Error: 'code' argument is missing or empty."
            return mcp_pb2.ToolRunResponse(result=result_value)

        # Use io.StringIO to capture the output of the exec call
        stdout_capture = io.StringIO()

        try:
            # Create a sandboxed environment for the exec call
            # Whitelist safe built-in functions
            safe_builtins = {
                'abs', 'all', 'any', 'ascii', 'bin', 'bool', 'bytearray', 'bytes', 'callable',
                'chr', 'complex', 'dict', 'divmod', 'enumerate', 'filter', 'float', 'format',
                'frozenset', 'getattr', 'hasattr', 'hash', 'hex', 'id', 'int', 'isinstance',
                'issubclass', 'iter', 'len', 'list', 'map', 'max', 'min', 'next', 'oct', 'ord',
                'pow', 'print', 'range', 'repr', 'reversed', 'round', 'set', 'slice', 'sorted',
                'str', 'sum', 'tuple', 'type', 'zip'
            }
            # Create a dictionary of the actual built-in functions that are allowed
            allowed_builtins = {k: __builtins__.__dict__.get(k) for k in safe_builtins}

            def blocked_function(*args, **kwargs):
                """This function is called when a disallowed function is accessed."""
                raise PermissionError("Access to file system, imports, and other dangerous functions is disabled in the code interpreter. Use dedicated tools like 'file_reader' or 'file_writer'.")

            # Block dangerous functions by overriding them
            allowed_builtins['open'] = blocked_function
            allowed_builtins['__import__'] = blocked_function
            allowed_builtins['eval'] = blocked_function
            allowed_builtins['exec'] = blocked_function
            allowed_builtins['exit'] = blocked_function
            allowed_builtins['quit'] = blocked_function

            # The final environment for the exec call
            sandboxed_globals = {"__builtins__": allowed_builtins}


            # Redirect stdout to our string buffer and execute the code in the sandbox
            with contextlib.redirect_stdout(stdout_capture):
                exec(code_to_run, sandboxed_globals)

            # Get the captured output
            result_text = stdout_capture.getvalue()
            logging.info(f"Execution successful. Result length: {len(result_text)}")

        except Exception as e:
            # If there's an error during execution, format it nicely for the LLM
            result_text = f"Execution failed with error:\n{type(e).__name__}: {e}"
            logging.error(result_text)

        # The result must be wrapped in a struct_pb2.Value to be sent back.
        result_value = struct_pb2.Value()
        result_value.string_value = result_text

        return mcp_pb2.ToolRunResponse(result=result_value)

def serve(port):
    """
    Starts the gRPC server on the specified port.
    """
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    mcp_pb2_grpc.add_ToolServicer_to_server(CodeInterpreterTool(), server)
    server.add_insecure_port(f'[::]:{port}')
    server.start()
    # Write to stderr to avoid interfering with any potential stdio transport
    logging.info(f"Code Interpreter gRPC server started on port {port}")
    server.wait_for_termination()

if __name__ == '__main__':
    setup_logging("code_interpreter")
    parser = argparse.ArgumentParser(description='Code Interpreter Tool gRPC Server')
    parser.add_argument('--port', type=int, required=True, help='The port to listen on for gRPC requests.')
    args = parser.parse_args()
    serve(args.port)
