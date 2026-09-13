package user

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"github.com/twbworld/agent/dao"
	"github.com/twbworld/agent/global"
	"github.com/twbworld/agent/internal/llm"
	"github.com/twbworld/agent/model/common"
	"github.com/twbworld/agent/model/config"
	"github.com/twbworld/agent/model/enum"
)

// ==============================================================================
// Mock 依赖区域：解耦外部服务影响
// ==============================================================================

type mockLlmService struct {
	ChatFunc func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error)
}

func (m *mockLlmService) Chat(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
	if m.ChatFunc != nil {
		return m.ChatFunc(ctx, req)
	}
	return nil, nil
}

type mockMcpService struct {
	GetAvailableToolsWithClientFunc func() map[string][]mcp.Tool
	ExecuteToolFunc                 func(ctx context.Context, clientName string, toolName string, arguments jsontext.Value) (string, error)
}

func (m *mockMcpService) Close() error                                        { return nil }
func (m *mockMcpService) GetAvailableTools() []mcp.Tool                       { return nil }
func (m *mockMcpService) GetToolDescriptions() map[string]string              { return nil }
func (m *mockMcpService) AddOrUpdateClient(name string, cfg config.Mcp) error { return nil }
func (m *mockMcpService) RemoveClient(name string) error                      { return nil }

func (m *mockMcpService) GetAvailableToolsWithClient() map[string][]mcp.Tool {
	if m.GetAvailableToolsWithClientFunc != nil {
		return m.GetAvailableToolsWithClientFunc()
	}
	return nil
}

func (m *mockMcpService) ExecuteTool(ctx context.Context, clientName string, toolName string, arguments jsontext.Value) (string, error) {
	if m.ExecuteToolFunc != nil {
		return m.ExecuteToolFunc(ctx, clientName, toolName, arguments)
	}
	return "", nil
}

// 辅助函数，用于快速创建指针
//
//go:fix inline
func stringPtr(s string) *string {
	return new(s)
}

// ==============================================================================
// 单元测试区域
// ==============================================================================

func TestLlmService_cleanJSON(t *testing.T) {
	svc := &llmService{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"无任何标记", `{"a": 1}`, `{"a": 1}`},
		{"包含 markdown json 标签", "```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"仅包含普通 markdown 标签", "```\n{\"a\": 1}\n```", `{"a": 1}`},
		{"包含多余空格的标签", "   ```json \n{\"a\": 1}\n```  ", `{"a": 1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.cleanJSON(tt.input)
			if result != tt.expected {
				t.Errorf("预期 %s, 但得到 %s", tt.expected, result)
			}
		})
	}
}

func TestLlmService_buildContextPrompt(t *testing.T) {
	svc := &llmService{}

	sender := common.Sender{
		Identifier: new("user123"),
		CustomAttributes: common.CustomAttributes{
			GoodsID: "g1",
			OrderID: "o1",
		},
	}

	prompt, err := svc.buildContextPrompt(sender)
	if err != nil {
		t.Fatalf("预期外错误: %v", err)
	}

	expectedParts := []string{
		"- goods_id: g1",
		"- order_id: o1",
		"- user_id: user123",
	}

	// 验证按顺序及拼凑正确的KV值
	for _, p := range expectedParts {
		if !strings.Contains(prompt, p) {
			t.Errorf("预期 Prompt 中包含 '%s', 但实际内容为:\n%s", p, prompt)
		}
	}
}

func TestLlmService_Triage(t *testing.T) {
	global.Log = logrus.New()
	global.Log.SetLevel(logrus.FatalLevel) // 屏蔽预期内的测试警告日志

	t.Run("成功响应并解析结构化数据", func(t *testing.T) {
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				return &openai.ChatCompletionMessage{
					Content: `{"intent": "product_inquiry", "emotion": "neutral", "urgency": "low", "answer_id": 1}`,
				}, nil
			},
		}
		global.LlmService = mockLlm

		svc := NewLlmService()
		res, err := svc.Triage(context.Background(), "hello", nil, []string{"q1"}, common.Sender{})
		if err != nil {
			t.Fatalf("预期外错误: %v", err)
		}
		if res.Intent != string(enum.TriageIntentProductInquiry) {
			t.Errorf("预期 intent=%s, 但得到 %v", enum.TriageIntentProductInquiry, res.Intent)
		}
		if res.AnswerID != 1 {
			t.Errorf("预期 AnswerID=1, 但得到 %v", res.AnswerID)
		}
	})

	t.Run("LLM 出现连接报错", func(t *testing.T) {
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				return nil, errors.New("llm mock error")
			},
		}
		global.LlmService = mockLlm

		svc := NewLlmService()
		_, err := svc.Triage(context.Background(), "hello", nil, []string{"q1"}, common.Sender{})
		if err == nil {
			t.Fatal("预期发生 LLM 调用报错, 实际未返回")
		}
	})
}

func TestLlmService_verifyToolResultOwnership(t *testing.T) {
	svc := &llmService{}

	sender := common.Sender{
		Identifier: new("user123"),
	}

	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{"合法的 user_id 匹配", `{"user_id": "user123", "data": "abc"}`, true},
		{"合法的 userId 匹配", `{"userId": "user123", "data": "abc"}`, true},
		{"非法的所有者 ID 匹配应阻断", `{"user_id": "user456", "data": "abc"}`, false},
		{"无 user id 标识，放行", `{"data": "abc"}`, true},
		{"返回结果非 JSON 格式，放行容错", `not a json string`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := svc.verifyToolResultOwnership(tt.raw, sender)
			if res != tt.expected {
				t.Errorf("预期 %v, 但得到 %v", tt.expected, res)
			}
		})
	}
}

func TestLlmService_ensureSafeArguments(t *testing.T) {
	svc := &llmService{}

	sender := common.Sender{
		Identifier: new("user123"),
	}

	toolDef := mcp.Tool{
		Name: "sensitive_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user_id":   map[string]any{"type": "string"},
				"other_arg": map[string]any{"type": "string"},
			},
		},
	}

	t.Run("成功注入安全鉴权用户ID", func(t *testing.T) {
		args := `{"other_arg": "abc"}`
		safeArgs, shouldExec, reason := svc.ensureSafeArguments(args, toolDef, sender)
		if !shouldExec {
			t.Fatalf("预期可以执行, 但得到拒绝原因: %s", reason)
		}

		var m map[string]any
		err := json.Unmarshal(safeArgs, &m)
		if err != nil {
			t.Fatalf("无法反序列化安全加工后的参数: %v", err)
		}
		if m["user_id"] != "user123" {
			t.Errorf("预期 user_id 被注入为 'user123', 但得到 %v", m["user_id"])
		}
	})

	t.Run("匿名用户触发敏感工具需拦截", func(t *testing.T) {
		args := `{"other_arg": "abc"}`
		_, shouldExec, reason := svc.ensureSafeArguments(args, toolDef, common.Sender{}) // 空的匿名会话
		if shouldExec {
			t.Fatal("预期应该拦截匿名用户，但通过了")
		}
		if reason == "" {
			t.Error("预期应返回不为空的安全拦截原因文案")
		}
	})
}

func TestLlmService_RunReActLoop(t *testing.T) {
	global.Log = logrus.New()
	global.Log.SetLevel(logrus.FatalLevel)
	global.Config.Ai.MaxReActRounds = 5
	global.Config.Ai.AgentTemperature = 0.5

	t.Run("普通对话模式 (不调用工具)", func(t *testing.T) {
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				return &openai.ChatCompletionMessage{
					Content: "我是AI智能助手",
				}, nil
			},
		}
		global.LlmService = mockLlm
		global.McpService = nil

		svc := NewLlmService()
		res, msgs, err := svc.RunReActLoop(context.Background(), &common.ChatRequest{Content: "你是谁?"}, nil, nil, common.Sender{})

		if err != nil {
			t.Fatalf("预期外报错: %v", err)
		}
		if res.TransferToHuman {
			t.Error("未触发相关关键词，预期转接人工为 false")
		}
		if res.Reply != "我是AI智能助手" {
			t.Errorf("预期回复内容为 '我是AI智能助手', 得到 '%s'", res.Reply)
		}
		if len(msgs) != 0 {
			t.Errorf("无中间工具交互，预期返回 0 个中间态会话，实际得到 %d", len(msgs))
		}
	})

	t.Run("携带 RAG 参考资料执行", func(t *testing.T) {
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				// 获取 RAG 上下文拼装在队尾的内容
				prompt := req.Messages[len(req.Messages)-1].Content
				if !strings.Contains(prompt, "RAG_ANSWER_DOC") {
					t.Errorf("预期的提示词中未发现注入的 RAG 资料")
				}
				return &openai.ChatCompletionMessage{
					Content: "根据资料回答",
				}, nil
			},
		}
		global.LlmService = mockLlm
		global.McpService = nil

		svc := NewLlmService()
		docs := []dao.SearchResult{
			{Question: "rag_question", Answer: "RAG_ANSWER_DOC"},
		}
		res, _, err := svc.RunReActLoop(context.Background(), &common.ChatRequest{Content: "问题"}, docs, nil, common.Sender{})

		if err != nil {
			t.Fatalf("预期外报错: %v", err)
		}
		if res.Reply != "根据资料回答" {
			t.Errorf("回答偏离: %s", res.Reply)
		}
	})

	t.Run("大模型主动决策调用退让逻辑转人工", func(t *testing.T) {
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				return &openai.ChatCompletionMessage{
					ToolCalls: []openai.ToolCall{
						{
							Function: openai.FunctionCall{
								Name: string(enum.ToolTransferToHuman),
							},
						},
					},
				}, nil
			},
		}
		global.LlmService = mockLlm
		global.McpService = nil

		svc := NewLlmService()
		res, _, err := svc.RunReActLoop(context.Background(), &common.ChatRequest{Content: "转人工"}, nil, nil, common.Sender{})

		if err != nil {
			t.Fatalf("预期外报错: %v", err)
		}
		if !res.TransferToHuman {
			t.Error("预期 TransferToHuman 应被设置为 true")
		}
	})

	t.Run("执行标准的 ReAct 思考 - 工具调用 - 返回 观察结果 轮循流", func(t *testing.T) {
		callCount := 0
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				callCount++
				if callCount == 1 {
					// 第一轮决策，AI选择调用工具查订单
					return &openai.ChatCompletionMessage{
						ToolCalls: []openai.ToolCall{
							{
								ID: "call_mcp_1",
								Function: openai.FunctionCall{
									Name:      "testClient_queryData",
									Arguments: `{}`,
								},
							},
						},
					}, nil
				}
				// 第二轮，携带查询结果(Tool Message)，AI给出最终判断和回复
				return &openai.ChatCompletionMessage{
					Content: "这是您请求的数据",
				}, nil
			},
		}
		global.LlmService = mockLlm

		mockMcp := &mockMcpService{
			GetAvailableToolsWithClientFunc: func() map[string][]mcp.Tool {
				return map[string][]mcp.Tool{
					"testClient": {
						{Name: "queryData"},
					},
				}
			},
			ExecuteToolFunc: func(ctx context.Context, clientName string, toolName string, arguments jsontext.Value) (string, error) {
				// MCP 模拟执行返回
				return `{"result": "success"}`, nil
			},
		}
		global.McpService = mockMcp

		svc := NewLlmService()
		res, msgs, err := svc.RunReActLoop(context.Background(), &common.ChatRequest{Content: "帮我查询下"}, nil, nil, common.Sender{})

		if err != nil {
			t.Fatalf("预期外报错: %v", err)
		}
		if res.TransferToHuman {
			t.Error("预期的普通数据调阅不应转接人工")
		}
		if res.Reply != "这是您请求的数据" {
			t.Errorf("预期得到了 LLM 第二轮给出的组装结果，实际得到 '%s'", res.Reply)
		}

		// ReAct 会将 (Assistant的调度决议 + Tool执行观察的返回结果) 共 2 条组装成 Intermediate Msgs 返回以便写入持久化记忆
		if len(msgs) != 2 {
			t.Fatalf("预期截获两条中间思考记录, 得到 %d 条", len(msgs))
		}
		if msgs[0].Role != openai.ChatMessageRoleAssistant {
			t.Errorf("第一条中间态应该是 assistant，但得到 %s", msgs[0].Role)
		}
		if msgs[1].Role != openai.ChatMessageRoleTool {
			t.Errorf("第二条中间态应该是 tool 结果响应，但得到 %s", msgs[1].Role)
		}
		if msgs[1].Content != `{"result": "success"}` {
			t.Errorf("工具响应载荷未被捕捉, 当前记录为 %s", msgs[1].Content)
		}
	})

	t.Run("ReAct 出现死循环或模型短路降级保护测试 (Max Loop Limit)", func(t *testing.T) {
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				return &openai.ChatCompletionMessage{
					// 持续产生无聊的不停止执行 Tool 的循环流
					ToolCalls: []openai.ToolCall{
						{
							ID: "call_infinite_loop",
							Function: openai.FunctionCall{
								Name:      "testClient_queryData",
								Arguments: `{}`,
							},
						},
					},
				}, nil
			},
		}
		global.LlmService = mockLlm

		mockMcp := &mockMcpService{
			GetAvailableToolsWithClientFunc: func() map[string][]mcp.Tool {
				return map[string][]mcp.Tool{
					"testClient": {{Name: "queryData"}},
				}
			},
			ExecuteToolFunc: func(ctx context.Context, clientName string, toolName string, arguments jsontext.Value) (string, error) {
				return `{"data": "loop"}`, nil
			},
		}
		global.McpService = mockMcp

		svc := NewLlmService()
		res, _, err := svc.RunReActLoop(context.Background(), &common.ChatRequest{Content: "死循环触发"}, nil, nil, common.Sender{})

		if err != nil {
			t.Fatalf("预期外报错: %v", err)
		}
		if !res.TransferToHuman {
			t.Error("当大模型抽风或持续超轮训时，代码逻辑的限制阀(5轮跳出)应当强行判定为退回转人工兜底")
		}
	})

	t.Run("MCP Tool 受到请求人凭证与归属数据不符带来的安全阻隔测试", func(t *testing.T) {
		callCount := 0
		mockLlm := &mockLlmService{
			ChatFunc: func(ctx context.Context, req *llm.ChatRequest) (*openai.ChatCompletionMessage, error) {
				callCount++
				if callCount == 1 {
					return &openai.ChatCompletionMessage{
						ToolCalls: []openai.ToolCall{
							{
								ID: "call_err",
								Function: openai.FunctionCall{
									Name:      "testClient_queryData",
									Arguments: `{}`,
								},
							},
						},
					}, nil
				}
				// 模拟 LLM 观察到了 Tool 返回了越权和安全阻隔，于是回复
				return &openai.ChatCompletionMessage{
					Content: "安全审查阻隔执行",
				}, nil
			},
		}
		global.LlmService = mockLlm

		mockMcp := &mockMcpService{
			GetAvailableToolsWithClientFunc: func() map[string][]mcp.Tool {
				return map[string][]mcp.Tool{
					"testClient": {{Name: "queryData"}},
				}
			},
			ExecuteToolFunc: func(ctx context.Context, clientName string, toolName string, arguments jsontext.Value) (string, error) {
				// MCP 模拟：查询到了与当前发起者(my_user_id) 不同的数据 owner(other_user_666) 的敏感负载
				return `{"user_id": "other_user_666", "data": "stolen"}`, nil
			},
		}
		global.McpService = mockMcp

		svc := NewLlmService()
		sender := common.Sender{Identifier: new("my_user_id")}
		_, msgs, err := svc.RunReActLoop(context.Background(), &common.ChatRequest{Content: "越权测试"}, nil, nil, sender)

		if err != nil {
			t.Fatalf("预期外报错: %v", err)
		}
		if len(msgs) < 2 {
			t.Fatalf("预期捕捉至少 2 条消息流, 得到 %d 条", len(msgs))
		}

		if !strings.Contains(msgs[1].Content, "安全拦截") {
			t.Errorf("未观察到数据隔离网兜底，当前拦截记录为: %s", msgs[1].Content)
		}
	})
}
