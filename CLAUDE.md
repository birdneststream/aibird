# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building
```bash
# Build the application (requires CGO for SQLite3)
CGO_ENABLED=1 go build -o aibird .

# Build with module tidying
go mod tidy && CGO_ENABLED=1 go build -o aibird .

# Cross-compile for Linux (requires CGO)
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o aibird .

# Note: SQLite3 requires CGO to be enabled. Ensure you have:
# - A C compiler (gcc/clang) installed
# - CGO_ENABLED=1 (default on most systems)
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./queue
```

### Code Quality
```bash
# Format code
go fmt ./...

# Run go vet for static analysis
go vet ./...

# Tidy dependencies
go mod tidy
```

### Database Debugging
```bash
# Connect to SQLite database for debugging
sqlite3 bird.db

# Useful SQL queries for debugging:
# View all networks
SELECT * FROM networks;

# View all channels for a network
SELECT * FROM channels WHERE network_id = (SELECT id FROM networks WHERE name = 'networkname');

# View all users for a network
SELECT * FROM irc_users WHERE network_id = (SELECT id FROM networks WHERE name = 'networkname');

# View user-channel relationships
SELECT u.nickname, c.name as channel, uc.joined_at 
FROM user_channels uc 
JOIN irc_users u ON uc.user_id = u.id 
JOIN channels c ON uc.channel_id = c.id 
JOIN networks n ON u.network_id = n.id 
WHERE n.name = 'networkname';

# View command leaderboard for a network
SELECT nickname, command, count FROM command_leaderboard 
WHERE network = 'networkname' 
ORDER BY count DESC LIMIT 10;

# View user modes
SELECT u.nickname, c.name as channel, um.modes 
FROM user_modes um 
JOIN irc_users u ON um.user_id = u.id 
JOIN channels c ON um.channel_id = c.id 
JOIN networks n ON u.network_id = n.id 
WHERE n.name = 'networkname';
```

### Deployment
```bash
# Deploy to remote server (requires SSH access)
./deploy.sh
```

## High-Level Architecture

### Core System Components

**Dual GPU Queue System** (`queue/`)
- Manages workload distribution between RTX 4090 and RTX 2070 GPUs
- `DualQueue` coordinates two separate queues with intelligent routing based on model requirements and user permissions
- Automatic fallback from 4090 to 2070 when primary GPU is unavailable
- Priority queuing for privileged users (admin/owner/high access level)

**IRC State Management** (`irc/state/`)
- Comprehensive tracking of IRC network state including users, channels, and server connections
- Thread-safe state operations with mutex protection
- Maintains user modes, channel membership, and network-specific configurations
- Handles reconnection logic and state restoration

**Command Dispatch System** (`irc/commands/`)
- Permission-based command execution with hierarchical access levels
- Commands are categorized by required permissions: owner, admin, or standard user
- Dynamic command loading from ComfyUI workflows in `comfyuijson/` directory
- Built-in flood protection and rate limiting per user

### Service Integration

**Text Generation** (`text/`)
- Multiple provider support: GLM, Gemini, LlamaCpp
- Provider abstraction through common interfaces
- Response caching system to reduce API calls
- Automatic provider selection based on model availability

**Image Generation** (`image/comfyui/`)
- ComfyUI integration using Comfy2Go library
- Workflow-based image generation from JSON files in `comfyuijson/`
- Each workflow contains metadata in `aibird_meta` node defining access levels and GPU requirements
- Automatic parameter mapping from IRC commands to workflow nodes

**File Upload Service** (`http/uploaders/birdhole/`)
- Integration with Birdhole gallery service for file sharing
- Automatic upload of generated images, audio, and video files
- Returns shareable URLs for IRC distribution

### Configuration System

**Settings Management** (`settings/`)
- TOML-based configuration split across multiple files:
  - `config.toml` - Main IRC networks and bot configuration
  - `settings/*.toml` - Service-specific configurations (birdhole, comfyui, gemini, etc.)
- Runtime configuration reloading capability
- Hierarchical configuration with defaults and overrides

### Database Layer (`birdbase/`)
- SQLite-based normalized database with proper relational schema
- Eliminated JSON blob storage in favor of structured columns and tables
- User statistics, access levels, and command leaderboard tracking
- Network-isolated data with proper foreign key relationships
- Thread-safe operations with proper locking and transactions

### Critical Design Patterns

**State Synchronization**
- All IRC state modifications go through centralized state manager
- Event-driven updates propagated through callback system
- Consistent state across multiple network connections

**Queue Processing**
- Asynchronous job processing with status callbacks
- Graceful degradation when services unavailable
- Timeout handling and automatic retry logic

**Security Model**
- Three-tier access system: Owner > Admin > User
- Per-command permission requirements
- Content filtering and moderation capabilities
- Flood protection with configurable thresholds

## Important Implementation Details

- The bot maintains persistent connections to multiple IRC networks simultaneously
- Each network connection runs in its own goroutine with independent state
- ComfyUI workflows must include `aibird_meta` node in API group for proper integration
- Generated files are temporarily stored locally before upload to Birdhole
- The system uses context-based cancellation for graceful shutdown
- All logging goes through centralized logger with configurable levels

### Database Schema Migration (Version 2)

The system has migrated from JSON blob storage to a fully normalized SQLite schema:

**Core Tables:**
- `networks` - IRC network configurations and runtime data
- `channels` - Channel settings with network foreign keys
- `irc_users` - User data with ident+host uniqueness per network
- `user_channels` - Many-to-many relationship tracking channel membership
- `user_modes` - User IRC modes per channel (voice, op, etc.)
- `servers` - Network server configurations
- `admin_hosts` - Authorized admin host patterns
- `ignored_nicks` - Network-specific ignored nicknames
- `denied_commands` - Network-specific command restrictions
- `command_leaderboard` - Permanent command usage tracking

**Key Design Decisions:**
- Configuration merge strategy: `config.toml` is source of truth, database stores runtime state
- User deduplication by `ident@host` within each network (handles nick changes)
- Foreign key constraints with cascading deletes for data integrity
- Individual user saves instead of full network saves for performance during netsplits
- Mode preservation respects channel and network `preserve_modes` settings

## Working with ComfyUI Workflows

When adding new image generation commands:
1. Place workflow JSON in `comfyuijson/` directory
2. Ensure workflow contains `aibird_meta` node with TOML configuration
3. Map command arguments to workflow node parameters in metadata
4. Set appropriate access levels and GPU requirements in metadata

## Go Best Practices for This Codebase

### Code Style and Conventions
- Use meaningful variable and function names that reflect IRC/bot domain terminology
- Follow standard Go naming conventions: exported types/functions use CamelCase, unexported use camelCase
- Keep functions focused and small - if a function exceeds 50 lines, consider refactoring
- Use early returns to reduce nesting and improve readability

### Error Handling
- Always check and handle errors explicitly - never ignore error returns
- Use wrapped errors with context: `fmt.Errorf("failed to connect to %s: %w", network, err)`
- Log errors at appropriate levels using the centralized logger
- For IRC commands, send user-friendly error messages back to the channel

### Concurrency Patterns
- Use goroutines for each IRC network connection
- Protect shared state with mutexes (see `state.Mutex` usage pattern)
- Use channels for communication between goroutines where appropriate
- Always use context for cancellation and timeout control

### Interface Design
- Define small, focused interfaces (e.g., `UserAccess` interface in queue system)
- Accept interfaces, return concrete types
- Use interface segregation - don't force implementations to depend on methods they don't use

### Testing Approach
- Write table-driven tests for functions with multiple test cases
- Use mock implementations for interfaces (see `MockUser` in queue tests)
- Test concurrent code with race detector: `go test -race`
- Focus tests on behavior rather than implementation details

### Performance Considerations
- Use sync.Pool for frequently allocated temporary objects
- Prefer bytes.Buffer over string concatenation in loops
- Cache expensive computations (see text response caching)
- Profile before optimizing - don't guess at performance issues

### Package Organization
- Keep packages focused on a single responsibility
- Avoid circular dependencies between packages
- Use internal packages for code that shouldn't be imported by external projects
- Group related functionality (e.g., all IRC-related code under `irc/`)

### Documentation
- Document all exported types, functions, and methods
- Include usage examples in comments for complex functions
- Document why, not what - the code shows what it does
- Keep comments up-to-date when code changes

### Specific to This Codebase
- When modifying IRC state, always go through the state manager
- Check user permissions before executing privileged commands
- Use the configured logger rather than fmt.Print or log packages directly
- Respect the queue system's GPU allocation logic
- Handle IRC disconnections gracefully with automatic reconnection
- Validate and sanitize all user input from IRC messages
- Use TOML tags consistently for configuration structs

### Database Operations
- Always use transactions for multi-table operations
- Use `SaveSingleUser()` for individual user updates during high-traffic events
- Call `network.Save()` to sync configuration changes from `config.toml` 
- Use `ON CONFLICT` clauses instead of `INSERT OR REPLACE` to avoid ID jumping
- Check `ShouldPreserveModes()` before applying user modes
- User identification uses `ident@host` matching for consistency across nick changes

### New Commands Added
- `!leaderboard` - Shows network-specific command usage leaderboard
- `!leaderboard global` - Shows global leaderboard (owner only)
- `!leaderboard <command>` - Shows command-specific leaderboard (owner only)
- Enhanced `!sync` command with mode restoration via WHO/RPL_ENDOFWHO handling
- `!dbstats` updated for SQLite database statistics