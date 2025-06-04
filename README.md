# MCPHOST-SERVER

A server bridging solution for MCPHOST that enables easy communication with local LLM (Ollama) through HTTP requests. This project is originated from [mark3labs/mcphost](https://github.com/mark3labs/mcphost) and adds server capabilities for enhanced functionality.


## Added Features

- Server mode for HTTP-based communication with local LLM for interactive requests
- Environment-based configuration

## Getting Started

1. Clone the repository

2. Copy the example environment file and configure it, see .env for configuration in details:
   ```bash
   cp .example.env .env
   ```

3. Build the project:
   ```bash
   go build
   ```

4. Run the server:
   ```bash
   ./mcphost-server
   ```

## Usage

### Server Mode

The server mode is enabled by default and provides HTTP endpoints for interacting with the local LLM. The server will start on the configured port (default: 8115).

### Command Line Mode

To use the command-line interface, set `server_mode = false` in the main.go file and rebuild the project.

## Configuration

The application can be configured through:
- Environment variables (see `.example.env`)
- `mcp.json` configuration file
- Command-line flags
