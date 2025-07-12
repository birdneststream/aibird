# AI Bird IRC Bot 🐦

A sophisticated multi-network IRC bot that provides AI-powered services including text generation, image generation, and audio processing. Built with Go and featuring a dual-GPU queue system, comprehensive state management, and modular architecture.

You can find the bot in use on popular IRC networks in the channel #birdnest. The main #birdnest channel is on efnet, however also on libera, rizon and a few other networks. It's free to use and so far compared to other online AI generation services for images, sounds or music - it has unlimited use and is less censored. Also as it's mostly ran from my local system here it's more private. The exception is when Google Gemini or OpenRouter services are used. Everything else runs off the `airig` which has a 2070 and 4090 dual GPU setup which is networked with wireguard.



While it is possible to run this yourself it may need some adjustments to how it works. Some parts are also incomplete and the full use of `aibird` is not properly documented.

The `aibird` also shares some other projects to make it functional.

* [birdcheck](https://github.com/birdneststream/birdcheck) - Birdcheck runs on the `airig` and provides a basic api to check the online status, gpu information and voices for the `!tts` command.
* [birdhole](https://github.com/birdneststream/birdhole) - The File upload and gallery service, which `aibird` uploads to for sharing the requested generations over IRC.

## AI Disclaimer

This bot is a mix of maual coding (70%) with autocomplete, and LLM agent assisted coding (30%).

## 🚀 Features

- **Multi-Network IRC Support** - Connect to multiple IRC networks simultaneously
- **AI-Powered Services** - Text generation, image generation, and audio processing
- **Dual GPU Queue System** - Intelligent routing between RTX 4090 and RTX 2070
- **Comprehensive State Management** - Robust IRC state tracking and user management
- **Security & Moderation** - Flood protection, content filtering, and access control
- **Graceful Shutdown** - Proper cleanup and resource management
- **Extensive Logging** - Detailed logging for debugging and monitoring

## 🏗️ Architecture

The bot is built with a modular architecture featuring:

- **Main Application** (`main.go`) - Entry point and orchestration
- **Configuration Management** (`settings/`) - TOML-based configuration
- **Dual Queue System** (`queue/`) - Intelligent GPU resource management
- **IRC State Management** (`irc/state/`) - Comprehensive IRC state tracking
- **Command System** (`irc/commands/`) - Permission-based command handling
- **Image Processing** (`image/comfyui/`) - Advanced image generation
- **Text Services** (`text/`) - Multiple AI text generation providers

## 📋 Requirements

- **Go 1.23+** - Minimum Go version required
- **GPU Support** - NVIDIA GPUs for image generation (optional)
- **IRC Networks** - Access to IRC networks you want to connect to
- **AI Services** - API keys for text generation services (OpenRouter, Gemini, etc.)

## 🛠️ Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/aibird.git
   cd aibird
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Build the application**
   ```bash
   go build -o aibird .
   ```

4. **Configure the bot**
   ```bash
   cp config.toml.example config.toml
   # Edit config.toml with your settings
   ```

5. **Run the bot**
   ```bash
   ./aibird
   ```

## ⚙️ Configuration

The bot uses TOML configuration files:

- **`config.toml`** - Main configuration file
- **`settings/`** - Service-specific configuration files

### Example Configuration

```toml
[networks.freenode]
enabled = true
nick = "aibird"
user = "aibird"
name = "AI Bird Bot"
channels = [
    { name = "#test", ai = true, sd = true }
]

[aibird]
actionTrigger = "!"
floodThreshold = 5
floodIgnoreMinutes = 10
```

## 🔧 Development

### Project Structure

```
aibird/
├── main.go              # Application entry point
├── config.toml          # Main configuration (gitignored)
├── settings/            # Configuration management
├── queue/               # Dual GPU queue system
├── irc/                 # IRC client and state management
├── image/               # Image generation services
├── text/                # Text generation services
├── http/                # HTTP utilities
├── logger/              # Logging system
├── helpers/             # Utility functions
└── shared/              # Shared components
```

## ComfyUi Workflows

Each ComfyUi workflow must be placed in the `comfyuiworkflows` directory for `aibird` to reconize as a command. Each workflow must also have a special API group, and in the group contains a text node `aibird_meta` which has some TOML information on how the workflow is used and what arguments override what node widget values.

An example workflow is provided as `sd-example.json`.

### Building

```bash
# Build for current platform
go build -o aibird .

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o aibird .
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./queue
```

### Code Quality

```bash
# Run go vet
go vet ./...

# Format code
go fmt ./...

# Tidy dependencies
go mod tidy
```

## 📚 Documentation

The project includes comprehensive inline documentation and comments throughout the codebase. Key areas to explore:

- **Main Application** (`main.go`) - Entry point with detailed comments
- **Configuration** (`settings/`) - Configuration management and validation
- **IRC System** (`irc/`) - IRC client, state management, and commands
- **Queue System** (`queue/`) - Dual GPU queue management
- **AI Services** (`text/`, `image/`) - Text and image generation services
- **Utilities** (`helpers/`, `logger/`) - Helper functions and logging

For detailed architecture information, check the inline comments and code structure.

## 🔒 Security

The bot includes several security features:

- **Input Validation** - All user inputs are validated
- **Access Control** - Hierarchical permission system
- **Content Filtering** - Configurable content moderation
- **Flood Protection** - Rate limiting and abuse prevention
- **Secure Configuration** - Sensitive files excluded from git

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Support

For support and questions:

- Create an issue on GitHub
- Review the inline code documentation
- Check the configuration examples

## 🙏 Acknowledgments

- Built with [Go](https://golang.org/)
- IRC client using [girc](https://github.com/lrstanley/girc)
- Image generation via [ComfyUI](https://github.com/comfyanonymous/ComfyUI) using [Comfy2Go](https://github.com/richinsley/comfy2go)
- Configuration using [TOML](https://github.com/BurntSushi/toml)

---

**AI Bird IRC Bot** - Bringing AI to IRC since 2022! 🐦✨ 