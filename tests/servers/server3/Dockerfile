FROM python:3.12-slim-bookworm

WORKDIR /app

RUN pip install fastmcp
COPY *.py /app

# CMD ["fastmcp run server.py --host 0.0.0.0 --port 9090 --transport http"]
CMD ["fastmcp", "run", "server.py", "--host", "0.0.0.0", "--port", "9090", "--transport", "http"]
