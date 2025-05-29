# Development Container for Go WhatsApp API

This development container provides a complete Go development environment with all necessary tools and dependencies pre-installed.

## What's Included

### Go Development Tools
- Go 1.24.3
- Air (hot reload)
- Delve debugger
- gopls (Go language server)
- golangci-lint
- staticcheck
- Swagger generator (swag)

### Additional Tools
- Git, curl, wget, jq
- SQLite3
- Docker in Docker
- ZSH with Oh My Zsh

### VS Code Extensions
- Go extension with full language support
- Docker extension
- GitHub Copilot (if available)
- Thunder Client for API testing
- And more...

## Services

The devcontainer includes:
- **Main App**: Your Go WhatsApp API application
- **RabbitMQ**: Message broker with management UI at http://localhost:15672
  - Username: `admin`
  - Password: `admin123`

## Getting Started

1. Make sure you have VS Code with the Remote-Containers extension installed
2. Open the project folder in VS Code
3. When prompted, click "Reopen in Container" or run "Remote-Containers: Reopen in Container" from the command palette
4. Wait for the container to build and start
5. The development server will start automatically with hot reload

## Available Tasks

Access these via `Ctrl+Shift+P` > "Tasks: Run Task":

- **Run with Air (Hot Reload)**: Starts the development server with automatic restart
- **Build**: Compiles the application
- **Test**: Runs all tests
- **Run Tests with Coverage**: Runs tests with coverage report
- **Lint**: Runs golangci-lint
- **Generate Swagger Docs**: Generates API documentation
- **Clean**: Removes temporary files

## Debugging

The devcontainer includes several debug configurations:
- **Launch API Server**: Debug the main application
- **Debug Current Test**: Debug the test file you're currently editing
- **Debug Current File**: Debug any Go file
- **Attach to Process**: Attach debugger to running process

## Ports

The following ports are forwarded to your host machine:
- `8080`: WhatsApp API server
- `5672`: RabbitMQ AMQP
- `15672`: RabbitMQ Management UI

## Environment Variables

Default development environment variables are set in the launch configuration. You can override them by creating a `.env` file in the project root.

## File Structure

The container excludes certain directories from VS Code's file watcher for better performance:
- `tmp/` (build artifacts)
- `sessions/` (WhatsApp session data)
- `data/` (database files)

## Tips

1. The Go module cache is persisted in a Docker volume for faster rebuilds
2. VS Code extensions are also persisted in a volume
3. Use the integrated terminal for running commands
4. All Go tools are pre-installed and configured
5. Air will automatically restart the server when you make changes to Go files
