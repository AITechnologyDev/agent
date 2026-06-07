# Agent

A lightweight, zero-dependency terminal AI-agent for vibe-coding. 

The original idea was [TermCode](https://github.com/AITechnologyDev/termcode), but it has been archived due to the heavy and complex BubbleTea code. `Agent` is a complete rewrite built on pure Go standard library for maximum speed on weak hardware (Termux/ARM, low-end PCs).

## ⚡ Why is it faster than standard TUI agents?
* **No frameworks:** No BubbleTea, no Viper. Pure `stdlib`.
* **Streaming Tool-Calls:** Parses LLM tool arguments on the fly using `strings.Builder` (zero heap allocations).
* **Tiny Binary:** ~8MB thanks to stripped debug symbols.

## 📱 Installation (Termux / ARM64)
Just run the setup script. It will install pre-compiled Go binaries to save your CPU:
```bash
bash setup_termux.sh
```
Then simply run:
```bash
agent
```

## 🐧 Installation (Linux / Arch)
```bash
go build -ldflags="-s -w" -o agent
./agent
```

## ⚙️ Configuration
The agent uses `~/.config/agent/config.json`. If the file is missing, it starts with safe defaults.
```json
{
  "api_url": "http://localhost:1234/v1/chat/completions",
  "model_name": "qwen2.5-coder-7b-instruct",
  "api_key": ""
}
```

## License
MIT
