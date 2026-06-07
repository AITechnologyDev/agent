package main

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// ANSI color codes
const (
    colorReset  = "\033[0m"
    colorRed    = "\033[31m"
    colorGreen  = "\033[32m"
    colorYellow = "\033[33m"
    colorCyan   = "\033[36m"
    colorGray   = "\033[90m"
)

// --- OpenAI-compatible API Data Structures ---

type ChatMessage struct {
    Role       string     `json:"role"`
    Content    string     `json:"content,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"`
    Function FunctionCall `json:"function"`
}

type FunctionCall struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}

type ToolDefinition struct {
    Type     string      `json:"type"`
    Function FunctionDef `json:"function"`
}

type FunctionDef struct {
    Name        string       `json:"name"`
    Description string       `json:"description"`
    Parameters  ParamsSchema `json:"parameters"`
}

type ParamsSchema struct {
    Type       string          `json:"type"`
    Properties map[string]Prop `json:"properties"`
    Required   []string        `json:"required"`
}

type Prop struct {
    Type        string   `json:"type"`
    Description string   `json:"description"`
    Enum        []string `json:"enum,omitempty"`
}

type APIRequest struct {
    Model    string           `json:"model"`
    Messages []ChatMessage    `json:"messages"`
    Tools    []ToolDefinition `json:"tools,omitempty"`
    Stream   bool             `json:"stream"`
}

type APIResponse struct {
    Choices []Choice  `json:"choices"`
    Error   *APIError `json:"error,omitempty"`
}

type Choice struct {
    Delta Delta `json:"delta"`
}

type Delta struct {
    Role      string          `json:"role,omitempty"`
    Content   string          `json:"content,omitempty"`
    ToolCalls []DeltaToolCall `json:"tool_calls,omitempty"`
}

type DeltaToolCall struct {
    Index    *int          `json:"index,omitempty"`
    ID       string        `json:"id,omitempty"`
    Type     string        `json:"type,omitempty"`
    Function DeltaFunction `json:"function,omitempty"`
}

type DeltaFunction struct {
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments,omitempty"`
}

type APIError struct {
    Message string `json:"message"`
    Type    string `json:"type"`
}

// --- Configuration ---
type Config struct {
    APIURL    string `json:"api_url"`
    ModelName string `json:"model_name"`
    APIKey    string `json:"api_key"`
}

// --- Optimized accumulator for streaming tool_calls ---
type ToolCallAccum struct {
    ID       string
    Name     string
    ArgsJson strings.Builder
}

func getDefaultConfig() Config {
    return Config{
        APIURL:    "http://localhost:1234/v1/chat/completions",
        ModelName: "qwen2.5-coder-7b-instruct",
        APIKey:    "",
    }
}

func loadConfig() Config {
    configDir := filepath.Join(os.Getenv("HOME"), ".config", "agent")
    configPath := filepath.Join(configDir, "config.json")

    cfg := getDefaultConfig()

    data, err := os.ReadFile(configPath)
    if err != nil {
        return cfg
    }

    var userCfg Config
    if err := json.Unmarshal(data, &userCfg); err != nil {
        fmt.Fprintf(os.Stderr, "%s⚠ Warning: config.json parse error, using defaults.%s\n", colorYellow, colorReset)
        return cfg
    }

    if userCfg.APIURL != "" {
        cfg.APIURL = userCfg.APIURL
    }
    if userCfg.ModelName != "" {
        cfg.ModelName = userCfg.ModelName
    }
    cfg.APIKey = userCfg.APIKey

    return cfg
}

func main() {
    cfg := loadConfig()

    history := []ChatMessage{
        {
            Role:    "system",
            // English prompts use fewer tokens, making the agent respond faster
            Content: "You are a fast and precise terminal AI assistant. Reply as briefly as possible. If code or a command needs to be executed in the terminal, use a tool_call. Do not write explanations before executing the command, just call the tool.",
        },
    }

    tools := []ToolDefinition{
        {
            Type: "function",
            Function: FunctionDef{
                Name:        "bash",
                Description: "Executes a command in the bash shell and returns the output.",
                Parameters: ParamsSchema{
                    Type: "object",
                    Properties: map[string]Prop{
                        "command": {
                            Type:        "string",
                            Description: "The command to execute in bash",
                        },
                    },
                    Required: []string{"command"},
                },
            },
        },
    }

    fmt.Printf("%s► Initializing Agent%s\n", colorCyan, colorReset)
    fmt.Printf("%s● Model: %s | Endpoint: %s%s\n", colorGray, cfg.ModelName, cfg.APIURL, colorReset)
    fmt.Printf("%s● Config: ~/.config/agent/config.json%s\n\n", colorGray, colorReset)

    scanner := bufio.NewScanner(os.Stdin)
    for {
        fmt.Printf("%s%s> %s", colorGreen, "You", colorReset)
        if !scanner.Scan() {
            break
        }
        userInput := strings.TrimSpace(scanner.Text())
        if userInput == "" {
            continue
        }
        if userInput == "exit" || userInput == "quit" {
            break
        }

        history = append(history, ChatMessage{Role: "user", Content: userInput})

        for {
            resp, toolCalls, err := streamChat(history, tools, cfg)
            if err != nil {
                fmt.Fprintf(os.Stderr, "\n%s✖ API Error: %v%s\n", colorRed, err, colorReset)
                history = history[:len(history)-1]
                break
            }

            if len(toolCalls) == 0 {
                history = append(history, ChatMessage{Role: "assistant", Content: resp})
                fmt.Println()
                break
            }

            var apiToolCalls []ToolCall
            var accumCalls []ToolCallAccum
            for _, tc := range toolCalls {
                apiToolCalls = append(apiToolCalls, ToolCall{
                    ID:   tc.ID,
                    Type: "function",
                    Function: FunctionCall{
                        Name:      tc.Name,
                        Arguments: tc.ArgsJson.String(),
                    },
                })
                accumCalls = append(accumCalls, tc)
            }
            history = append(history, ChatMessage{Role: "assistant", ToolCalls: apiToolCalls})

            for _, tc := range accumCalls {
                fmt.Printf("\n%s⚙ Tool Call [%s]%s\n", colorYellow, tc.Name, colorReset)
                cmdStr := extractJSONField(tc.ArgsJson.String(), "command")
                if cmdStr == "" {
                    cmdStr = tc.ArgsJson.String()
                }

                fmt.Printf("%s$ %s%s\n", colorGray, cmdStr, colorReset)

                out, err := exec.Command("bash", "-c", cmdStr).CombinedOutput()
                result := string(out)
                if err != nil {
                    result = fmt.Sprintf("[ERROR] %s\n%s", err.Error(), result)
                }

                if len(result) > 2000 {
                    result = result[:2000] + "\n...[output truncated by agent]"
                }

                fmt.Printf("%s%s%s\n", colorGray, result, colorReset)

                history = append(history, ChatMessage{
                    Role:       "tool",
                    Content:    result,
                    ToolCallID: tc.ID,
                })
            }
        }
    }
}

func streamChat(history []ChatMessage, tools []ToolDefinition, cfg Config) (string, []ToolCallAccum, error) {
    reqBody := APIRequest{
        Model:    cfg.ModelName,
        Messages: history,
        Tools:    tools,
        Stream:   true,
    }

    jsonData, err := json.Marshal(reqBody)
    if err != nil {
        return "", nil, fmt.Errorf("marshaling error: %w", err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIURL, bytes.NewBuffer(jsonData))
    if err != nil {
        return "", nil, err
    }
    req.Header.Set("Content-Type", "application/json")

    if cfg.APIKey != "" {
        req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        var errResp APIResponse
        json.NewDecoder(resp.Body).Decode(&errResp)
        if errResp.Error != nil {
            return "", nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error.Message)
        }
        return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
    }

    var contentBuilder strings.Builder
    var toolCallsAccum []ToolCallAccum

    scanner := bufio.NewScanner(resp.Body)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

    for scanner.Scan() {
        line := scanner.Text()
        if !strings.HasPrefix(line, "data: ") {
            continue
        }

        data := strings.TrimPrefix(line, "data: ")
        if data == "[DONE]" {
            break
        }

        var chunk APIResponse
        if err := json.Unmarshal([]byte(data), &chunk); err != nil {
            continue
        }

        if len(chunk.Choices) == 0 {
            continue
        }

        delta := chunk.Choices[0].Delta

        if delta.Content != "" {
            fmt.Print(delta.Content)
            contentBuilder.WriteString(delta.Content)
        }

        for _, tc := range delta.ToolCalls {
            idx := 0
            if tc.Index != nil {
                idx = *tc.Index
            }

            for len(toolCallsAccum) <= idx {
                toolCallsAccum = append(toolCallsAccum, ToolCallAccum{})
            }

            if tc.ID != "" {
                toolCallsAccum[idx].ID = tc.ID
            }
            if tc.Function.Name != "" {
                toolCallsAccum[idx].Name = tc.Function.Name
            }
            if tc.Function.Arguments != "" {
                toolCallsAccum[idx].ArgsJson.WriteString(tc.Function.Arguments)
            }
        }
    }

    return contentBuilder.String(), toolCallsAccum, scanner.Err()
}

func extractJSONField(jsonStr, key string) string {
    searchKey := `"` + key + `"`
    startIdx := strings.Index(jsonStr, searchKey)
    if startIdx == -1 {
        return ""
    }

    colonIdx := strings.Index(jsonStr[startIdx:], ":")
    if colonIdx == -1 {
        return ""
    }
    colonIdx += startIdx

    valStart := colonIdx + 1
    for valStart < len(jsonStr) && jsonStr[valStart] == ' ' {
        valStart++
    }

    if valStart >= len(jsonStr) {
        return ""
    }

    if jsonStr[valStart] == '"' {
        valStart++
        valEnd := strings.Index(jsonStr[valStart:], `"`)
        if valEnd == -1 {
            return ""
        }
        return jsonStr[valStart : valStart+valEnd]
    } else {
        valEnd := valStart
        for valEnd < len(jsonStr) && jsonStr[valEnd] != ',' && jsonStr[valEnd] != '}' && jsonStr[valEnd] != ']' {
            valEnd++
        }
        return strings.TrimSpace(jsonStr[valStart:valEnd])
    }
}