#!/bin/bash
cd "$(dirname "$0")/.."

echo "--- Starting Chaos Test ---"

# 1. Start Server
./MCP-NG/bin/server --transport=http --no-auth --log-file=server_chaos.log > server_stdout.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"
sleep 5

# 2. Start Load (Calculator)
echo "Starting background load..."
# Run for 20 seconds roughly (10k requests / 10 threads)
go run tests/load_tester/main.go -c 10 -n 10000 -scenario calc > chaos_load.log 2>&1 &
LOAD_PID=$!

# 3. Chaos Loop (Kill Calculator)
echo "Unleashing Chaos Monkey..."
for i in {1..5}; do
    sleep 2
    # Find tool process. Look for the binary running from MCP-NG/bin or tools/go
    # pgrep might find the 'go run' process too if compiling?
    # But server launches compiled binary 'calculator'.
    # We look for the child of the server?
    # Just killing by name 'calculator' is usually fine in container/clean env.
    TOOL_PID=$(pgrep -n calculator)
    if [ -n "$TOOL_PID" ]; then
        echo "Killing Calculator PID: $TOOL_PID"
        kill -9 $TOOL_PID
    else
        echo "Calculator process not found (maybe recovering?)"
    fi
done

# 4. Wait for load to finish
wait $LOAD_PID

# 5. Check Server
if ps -p $SERVER_PID > /dev/null; then
    echo "✅ Server SURVIVED the chaos."
    kill $SERVER_PID
else
    echo "❌ Server DIED during chaos."
fi

echo "--- Chaos Test Complete ---"
# Show last few lines of load log to see results
tail -n 20 chaos_load.log
