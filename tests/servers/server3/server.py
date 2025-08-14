"""
A simple MCP server for testing, implemented using the FastMCP library.
See https://github.com/jlowin/fastmcp
"""

from datetime import datetime
import time as pytime # If we don't rename this, it confuses fastmcp

from fastmcp import FastMCP, Context

mcp = FastMCP("FastMCP test server")

@mcp.tool
def time() -> str:
    """Return the current time."""
    return str(datetime.now())

@mcp.tool
def add(a: int, b: int) -> int:
    """Add two numbers"""
    return a + b

@mcp.tool
def dozen() -> int:
    """Return 12"""
    return 12

@mcp.tool
def pi() -> float:
    """Return 3.1415"""
    return 3.1415

@mcp.tool
def get_weather(city: str) -> dict:
    """Gets the current weather for a specific city."""
    # In a real app, this would call a weather API
    return {"city": city, "temperature": "72F", "forecast": "Sunny"}

@mcp.tool
async def slow(seconds: int, ctx: Context) -> str:
    """A long-running tool that waits N seconds, notifying the client of progress"""

    start_time = pytime.time()
    print(f"Slow tool will wait for {seconds} seconds")
    while True:
        waited = pytime.time() - start_time
        if waited >= seconds:
            break

        await ctx.report_progress(progress=int(waited), total=seconds)

        pytime.sleep(1)

    return ""

if __name__ == "__main__":
    # NOTE THIS NEVER GETS INVOKED.  WE RUN WITH THE FastMCP harness:
    # fastmcp run server.py --transport http
    print("Syntax: Use `fastmcp run server.py --transport http` instead.")
