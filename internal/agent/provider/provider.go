package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type ProviderState string

const (
	StateDisconnected ProviderState = "disconnected"
	StateConnecting   ProviderState = "connecting"
	StateReady        ProviderState = "ready"
	StateBusy         ProviderState = "busy"
	StateError        ProviderState = "error"
)

type AgentStatus struct {
	State     ProviderState `json:"state"`
	SessionID string        `json:"session_id,omitempty"`
	AgentName string        `json:"agent_name,omitempty"`
	Version   string        `json:"version,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type ToolCallback func(ctx context.Context, name string, args map[string]any) (CallToolResult, error)
type SessionStartCallback func(ctx context.Context, info ServerInfo) error

type ACPProvider interface {
	Initialize(ctx context.Context, clientName, clientVersion string) (*InitializeResult, error)
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*CallToolResult, error)
	CreateMessage(ctx context.Context, params CreateMessageParams) (*CreateMessageResult, error)
	GetStatus() AgentStatus
	OnToolCall(callback ToolCallback)
	OnSessionStart(callback SessionStartCallback)
	Close() error
}

type ACPProviderConfig struct {
	Name         string
	Version      string
	Instructions string
	Tools        []Tool
}

type BaseProvider struct {
	mu           sync.RWMutex
	state        ProviderState
	tools        []Tool
	toolCallback ToolCallback
	sessionStart SessionStartCallback
	status       AgentStatus
}

func NewBaseProvider(config ACPProviderConfig) *BaseProvider {
	return &BaseProvider{
		state: StateDisconnected,
		tools: config.Tools,
		status: AgentStatus{
			State:     StateDisconnected,
			AgentName: config.Name,
			Version:   config.Version,
		},
	}
}

func (p *BaseProvider) setState(state ProviderState) {
	p.mu.Lock()
	p.state = state
	p.status.State = state
	p.mu.Unlock()
}

func (p *BaseProvider) GetStatus() AgentStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *BaseProvider) OnToolCall(callback ToolCallback) {
	p.mu.Lock()
	p.toolCallback = callback
	p.mu.Unlock()
}

func (p *BaseProvider) OnSessionStart(callback SessionStartCallback) {
	p.mu.Lock()
	p.sessionStart = callback
	p.mu.Unlock()
}

func (p *BaseProvider) ListTools(ctx context.Context) ([]Tool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tools, nil
}

func (p *BaseProvider) AddTool(tool Tool) {
	p.mu.Lock()
	p.tools = append(p.tools, tool)
	p.mu.Unlock()
}

func (p *BaseProvider) RemoveTool(name string) {
	p.mu.Lock()
	for i, t := range p.tools {
		if t.Name == name {
			p.tools = append(p.tools[:i], p.tools[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
}

type LocalProvider struct {
	*BaseProvider
	instructions string
}

func NewLocalProvider(config ACPProviderConfig) *LocalProvider {
	return &LocalProvider{
		BaseProvider: NewBaseProvider(config),
		instructions: config.Instructions,
	}
}

func (p *LocalProvider) Initialize(ctx context.Context, clientName, clientVersion string) (*InitializeResult, error) {
	p.setState(StateReady)
	result := &InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    p.status.AgentName,
			Version: p.status.Version,
		},
		Instructions: p.instructions,
	}
	p.mu.RLock()
	callback := p.sessionStart
	p.mu.RUnlock()
	if callback != nil {
		if err := callback(ctx, result.ServerInfo); err != nil {
			return nil, fmt.Errorf("session start callback: %w", err)
		}
	}
	return result, nil
}

func (p *LocalProvider) CallTool(ctx context.Context, name string, args map[string]any) (*CallToolResult, error) {
	p.mu.RLock()
	callback := p.toolCallback
	p.mu.RUnlock()
	if callback == nil {
		return &CallToolResult{
			Content: []ContentBlock{NewTextContent("no tool callback registered")},
			IsError: true,
		}, nil
	}
	result, err := callback(ctx, name, args)
	if err != nil {
		return &CallToolResult{
			Content: []ContentBlock{NewTextContent(err.Error())},
			IsError: true,
		}, nil
	}
	return &result, nil
}

func (p *LocalProvider) CreateMessage(ctx context.Context, params CreateMessageParams) (*CreateMessageResult, error) {
	return nil, fmt.Errorf("CreateMessage not supported for local provider")
}

func (p *LocalProvider) Close() error {
	p.setState(StateDisconnected)
	return nil
}

func TranslateGastownMessage(from, to, subject, body string) Message {
	var content string
	if subject != "" && body != "" {
		content = fmt.Sprintf("**Subject:** %s\n\n%s", subject, body)
	} else if subject != "" {
		content = subject
	} else if body != "" {
		content = body
	}
	return NewUserMessage(content)
}

func ExtractToolCalls(msg Message) []ToolCallInfo {
	var calls []ToolCallInfo
	for _, block := range msg.Content {
		if block.Type == ContentTypeToolUse {
			calls = append(calls, ToolCallInfo{
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return calls
}

type ToolCallInfo struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func ExtractToolResults(msg Message) []ToolResultInfo {
	var results []ToolResultInfo
	for _, block := range msg.Content {
		if block.Type == ContentTypeToolResult {
			results = append(results, ToolResultInfo{
				ToolUseID: block.ToolUseID,
				Content:   block.Content,
				IsError:   block.IsError,
			})
		}
	}
	return results
}

type ToolResultInfo struct {
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content"`
	IsError   bool   `json:"is_error"`
}

func ExtractTextContent(msg Message) string {
	var text string
	for _, block := range msg.Content {
		if block.Type == ContentTypeText && block.Text != "" {
			if text != "" {
				text += "\n"
			}
			text += block.Text
		}
	}
	return text
}

func MessagesToJSON(msgs []Message) ([]byte, error) {
	return json.Marshal(msgs)
}

func MessagesFromJSON(data []byte) ([]Message, error) {
	var msgs []Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return msgs, nil
}

func RequestToJSON(req JSONRPCRequest) ([]byte, error) {
	return json.Marshal(req)
}

func ResponseToJSON(resp JSONRPCResponse) ([]byte, error) {
	return json.Marshal(resp)
}

func ResponseFromJSON(data []byte) (*JSONRPCResponse, error) {
	return ParseResponse(data)
}

func RequestFromJSON(data []byte) (*JSONRPCRequest, error) {
	return ParseRequest(data)
}

func NewInitializedNotification() JSONRPCRequest {
	return JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		Method:  "notifications/initialized",
	}
}

func IsNotification(req *JSONRPCRequest) bool {
	return req.ID == nil
}
