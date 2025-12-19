import argparse
import sys
import json
import os
import logging
from concurrent import futures
import grpc
from google.protobuf import struct_pb2

# Import shared protobuf modules
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
    # Determine log path relative to the tool execution directory
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

    # Stderr handler (Text)
    stderr_handler = logging.StreamHandler(sys.stderr)
    stderr_handler.setFormatter(logging.Formatter('%(asctime)s - %(name)s - %(levelname)s - %(message)s'))
    logger.addHandler(stderr_handler)

    logging.info(f"Logging initialized. Writing JSON to {log_file}")

class CalculatorTool(mcp_pb2_grpc.ToolServicer):
    """
    Implements the gRPC service for the Calculator tool (formerly FastAPI Calculator).
    """
    def GetDescription(self, request, context):
        """
        Returns the description of the tool.
        """
        schema = {
            "type": "object",
            "properties": {
                "expression": {
                    "type": "string",
                    "description": "The mathematical expression to evaluate."
                }
            },
            "required": ["expression"]
        }
        return mcp_pb2.ToolDescription(
            name="fastapi_calculator",
            description="Evaluate python math expression. Backup calculator.",
            input_schema_json=json.dumps(schema)
        )

    def Run(self, request, context):
        """
        Executes the tool with the provided arguments.
        """
        args = dict(request.arguments)
        expression = args.get("expression")

        logging.info(f"Run called with expression: {expression}")

        if not expression:
            logging.error("Error: 'expression' argument is missing.")
            result_value = struct_pb2.Value()
            result_value.string_value = "Error: 'expression' argument is missing."
            return mcp_pb2.ToolRunResponse(result=result_value)

        try:
            # Evaluate the expression safely
            # Note: eval() is dangerous if not restricted.
            # For this task, we are replicating the behavior of the original tool.
            # In a real production env, use a safer math parser.
            result = eval(expression, {"__builtins__": {}}, {})
            result_text = str(result)
            logging.info(f"Evaluation successful. Result: {result_text}")
        except Exception as e:
            result_text = f"Error evaluating expression: {e}"
            logging.error(result_text)

        result_value = struct_pb2.Value()
        result_value.string_value = result_text

        return mcp_pb2.ToolRunResponse(result=result_value)

def serve(port):
    """
    Starts the gRPC server on the specified port.
    """
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    mcp_pb2_grpc.add_ToolServicer_to_server(CalculatorTool(), server)
    server.add_insecure_port(f'[::]:{port}')
    server.start()
    logging.info(f"FastAPI Calculator (gRPC) server started on port {port}")
    server.wait_for_termination()

if __name__ == '__main__':
    setup_logging("fastapi_calculator")
    parser = argparse.ArgumentParser(description='Calculator Tool gRPC Server')
    parser.add_argument('--port', type=int, required=True, help='The port to listen on for gRPC requests.')
    args = parser.parse_args()
    serve(args.port)
