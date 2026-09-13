package user

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"
	"github.com/twbworld/agent/dao"
	"github.com/twbworld/agent/global"
	"github.com/twbworld/agent/internal/llm"
	"github.com/twbworld/agent/model/common"
	"github.com/twbworld/agent/model/enum"
	"github.com/twbworld/agent/service/admin"
	"golang.org/x/sync/errgroup"
)

type LlmService interface {
	Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string, sender common.Sender) (*common.TriageResult, error)
	// RunReActLoop 控制着LLM进行"思考-调用-观察"的多轮循环，并处理最终响应
	RunReActLoop(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage, sender common.Sender) (*common.LlmComplexResponse, []common.LlmMessage, error)
}

type llmService struct{}

func NewLlmService() LlmService {
	return &llmService{}
}

// 清洗并降级容错提取 JSON 的公共方法
func (s *llmService) cleanJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if after, ok := strings.CutPrefix(raw, "```json"); ok {
		raw = after
		raw = strings.TrimSuffix(raw, "```")
		return strings.TrimSpace(raw)
	} else if after, ok := strings.CutPrefix(raw, "```"); ok {
		raw = after
		raw = strings.TrimSuffix(raw, "```")
		return strings.TrimSpace(raw)
	}
	return raw
}

func (s *llmService) Triage(ctx context.Context, content string, history []common.LlmMessage, retrievedQuestions []string, sender common.Sender) (*common.TriageResult, error) {
	if global.LlmService == nil {
		return nil, fmt.Errorf("LLM客户端未初始化")
	}

	// 构建发送给小模型的prompt
	var finalContent strings.Builder

	contextPrompt, err := s.buildContextPrompt(sender)
	if err != nil {
		global.Log.Warnf("[Triage] 构建上下文提示词失败: %v", err)
	} else if contextPrompt != "" {
		finalContent.WriteString("--- 上下文信息 ---\n")
		finalContent.WriteString(contextPrompt)
		finalContent.WriteString("\n")
	}

	if len(retrievedQuestions) > 0 {
		finalContent.WriteString("--- 可能相关的问题 ---\n")
		for i, q := range retrievedQuestions {
			fmt.Fprintf(&finalContent, "%d. %s\n", i+1, q)
		}
		finalContent.WriteString("\n")
	}

	finalContent.WriteString("--- 用户最新问题 ---\n")
	finalContent.WriteString(content)

	triageSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"intent": map[string]any{
				"type": "string",
				"enum": []string{
					string(enum.TriageIntentProductInquiry),
					string(enum.TriageIntentOrderInquiry),
					string(enum.TriageIntentAfterSales),
					string(enum.TriageIntentRequestHuman),
					string(enum.TriageIntentOffTopic),
					string(enum.TriageIntentOtherInquiry),
				},
				"description": string(enum.TriageDescIntent),
			},
			"emotion": map[string]any{
				"type": "string",
				"enum": []string{
					string(enum.TriageEmotionAngry),
					string(enum.TriageEmotionFrustrated),
					string(enum.TriageEmotionAnxious),
					string(enum.TriageEmotionConfused),
					string(enum.TriageEmotionNeutral),
					string(enum.TriageEmotionPositive),
				},
				"description": string(enum.TriageDescEmotion),
			},
			"urgency": map[string]any{
				"type": "string",
				"enum": []string{
					string(enum.TriageUrgencyCritical),
					string(enum.TriageUrgencyHigh),
					string(enum.TriageUrgencyMedium),
					string(enum.TriageUrgencyLow),
				},
				"description": string(enum.TriageDescUrgency),
			},
			"answer_id": map[string]any{
				"type":        "integer",
				"description": string(enum.TriageDescAnswerID),
			},
		},
		"required":             []string{"intent", "emotion", "urgency", "answer_id"},
		"additionalProperties": false,
	}

	// 调用小模型,使用结构化输出
	messages := make([]common.LlmMessage, len(history), len(history)+1)
	copy(messages, history)
	messages = append(messages, common.LlmMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: finalContent.String(),
	})
	respMsg, err := global.LlmService.Chat(ctx, &llm.ChatRequest{
		Size:            enum.ModelSmall,
		SystemPrompt:    enum.SystemPromptTriage,
		Messages:        messages,
		SchemaName:      enum.SchemaNameTriage,
		Schema:          triageSchema,
		Strict:          true,
		Temperature:     new(global.Config.Ai.TriageTemperature),
		DisableThinking: true,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("分诊台LLM调用失败: %w", err)
	}

	cleanContent := s.cleanJSON(respMsg.Content)

	var triageResult common.TriageResult
	if err := json.Unmarshal([]byte(cleanContent), &triageResult); err != nil {
		return nil, fmt.Errorf("解析分诊台返回的JSON失败: %w, 原始返回: %s", err, respMsg.Content)
	}

	return &triageResult, nil
}

// 从 sender 对象构建上下文提示字符串
func (s *llmService) buildContextPrompt(sender common.Sender) (string, error) {
	attrMap, err := customAttributesToMap(sender.CustomAttributes)
	if err != nil {
		return "", fmt.Errorf("转换CustomAttributes为map失败: %w", err)
	}

	// 将 identifier 添加到 map 中以便统一处理
	if sender.Identifier != nil && *sender.Identifier != "" {
		attrMap["user_id"] = *sender.Identifier
	}

	if len(attrMap) == 0 {
		return "", nil
	}

	// 提取并排序 Keys，确保 Prompt 的确定性，从而提高 KV Cache 命中率
	keys := make([]string, 0, len(attrMap))
	for k := range attrMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var attrBuilder strings.Builder
	hasContent := false
	for _, key := range keys {
		value := attrMap[key]
		if value == nil {
			continue
		}
		if vStr, ok := value.(string); ok && vStr != "" {
			attrBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, vStr))
			hasContent = true
		} else if _, ok := value.(string); !ok {
			attrBuilder.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
			hasContent = true
		}
	}

	if !hasContent {
		return "", nil
	}

	return attrBuilder.String(), nil
}

func customAttributesToMap(attrs common.CustomAttributes) (map[string]any, error) {
	var attrMap map[string]any
	bytes, err := json.Marshal(attrs)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &attrMap)
	if err != nil {
		return nil, err
	}
	return attrMap, nil
}

type mappedTool struct {
	ClientName string
	ToolName   string
	Def        mcp.Tool
}

func (s *llmService) RunReActLoop(ctx context.Context, param *common.ChatRequest, referenceDocs []dao.SearchResult, history []common.LlmMessage, sender common.Sender) (*common.LlmComplexResponse, []common.LlmMessage, error) {
	if global.LlmService == nil {
		return nil, nil, fmt.Errorf("LLM客户端未初始化")
	}

	systemPrompt := enum.SystemPromptAgent

	var tools []openai.Tool
	toolsMap := make(map[string]mappedTool)

	if global.McpService != nil {
		toolMapByClient := global.McpService.GetAvailableToolsWithClient()

		// 提取 Key 并排序，保证顺序绝对一致，最大化命中 KV Cache
		clientNames := make([]string, 0, len(toolMapByClient))
		for cName := range toolMapByClient {
			clientNames = append(clientNames, cName)
		}
		sort.Strings(clientNames)

		for _, clientName := range clientNames {
			clientTools := toolMapByClient[clientName]
			// 对客户端内的工具名也进行排序
			sort.Slice(clientTools, func(i, j int) bool {
				return clientTools[i].Name < clientTools[j].Name
			})

			for _, tool := range clientTools {
				safeName := fmt.Sprintf("%s_%s", clientName, tool.Name)
				toolsMap[safeName] = mappedTool{ClientName: clientName, ToolName: tool.Name, Def: tool}

				var schema any = tool.InputSchema
				if schema == nil {
					schema = map[string]any{"type": "object", "properties": map[string]any{}}
				}

				tools = append(tools, openai.Tool{
					Type: openai.ToolTypeFunction, //这不能使用ToolTypeMCP
					Function: &openai.FunctionDefinition{
						Name:        safeName,
						Description: tool.Description,
						Parameters:  schema,
					},
				})
			}
		}
	}

	// 注入虚拟工具
	tools = append(tools, openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        string(enum.ToolTransferToHuman),
			Description: "当无法处理、用户要求、情绪激动或涉及高风险业务时，调用此工具转接人工客服。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reason": map[string]any{
						"type": "string",
						"enum": []string{
							string(enum.TransferToHuman1),
							string(enum.TransferToHuman4),
							string(enum.TransferToHuman5),
							string(enum.TransferToHuman6),
						},
						"description": "转接人工的具体原因",
					},
				},
				"required":             []string{"reason"},
				"additionalProperties": false, // 严格约束
			},
		},
	})

	// 优化点 2: 严格遵循静态 -> 动态的最佳排列次序: Context -> Docs -> Time -> Question
	var finalContent strings.Builder
	contextPrompt, err := s.buildContextPrompt(sender)
	if err != nil {
		global.Log.Warnf("[RunReActLoop] 构建上下文提示词失败: %v", err)
	} else if contextPrompt != "" {
		finalContent.WriteString("--- 上下文信息 ---\n")
		finalContent.WriteString(contextPrompt)
		finalContent.WriteString("\n")
	}

	if len(referenceDocs) > 0 {
		finalContent.WriteString("--- 参考资料 ---\n")
		for _, doc := range referenceDocs {
			if doc.Question == "" {
				doc.Question = "相关信息"
			}
			fmt.Fprintf(&finalContent, "[问题]: %s\n[回答]: %s\n---\n", doc.Question, doc.Answer)
		}
	}

	finalContent.WriteString("--- 当前系统时间 ---\n")
	finalContent.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	finalContent.WriteString("\n\n--- 用户最新问题 ---\n")
	finalContent.WriteString(param.Content)

	global.Log.Debugln("LLM系统提示词==========", systemPrompt)

	// 能够确保后续工具调用的 N 个回合之间始终保持绝对连贯且越来越长的 KV 前缀，实现 100% 缓存命中。
	// 根据实际轮数精准预分配容量，杜绝 ReAct 过程中的底层数组扩容
	// 容量 = 历史消息数 + 1条当前User问题 + 每轮至多产生2条消息(Assistant + Tool)
	currentHistory := make([]common.LlmMessage, len(history), len(history)+1+int(global.Config.Ai.MaxReActRounds)*2)
	copy(currentHistory, history)
	currentHistory = append(currentHistory, common.LlmMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: finalContent.String(),
	})

	var intermediateMsgs []common.LlmMessage

	// 开启标准的 ReAct 循环（设5轮兜底死循环跳出）
	for i := range global.Config.Ai.MaxReActRounds {
		global.Log.Debugf("开始第 %d 轮 ReAct 循环...", i+1)

		respMsg, err := global.LlmService.Chat(ctx, &llm.ChatRequest{
			Size:         enum.ModelLarge,
			SystemPrompt: systemPrompt,
			Messages:     currentHistory,
			Tools:        tools,
			Temperature:  new(global.Config.Ai.AgentTemperature),
		})
		if err != nil {
			return nil, intermediateMsgs, fmt.Errorf("调用LLM失败: %w", err)
		}

		//LLM指示 转人工
		for _, tc := range respMsg.ToolCalls {
			if tc.Function.Name == string(enum.ToolTransferToHuman) {
				global.Log.Debugf("[ReAct] LLM主动调用了转人工工具: %s", tc.Function.Arguments)

				var args struct {
					Reason string `json:"reason"`
				}
				reasonEnum := enum.TransferToHuman5
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil && args.Reason != "" {
					reasonEnum = enum.TransferToHuman(args.Reason)
				}

				return &common.LlmComplexResponse{
					TransferToHuman: true,
					TransferReason:  reasonEnum,
				}, intermediateMsgs, nil
			}
		}

		if len(respMsg.ToolCalls) == 0 {
			return &common.LlmComplexResponse{
				TransferToHuman: false,
				Reply:           respMsg.Content,
			}, intermediateMsgs, nil
		}

		assistantMsg := common.LlmMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   respMsg.Content,
			ToolCalls: respMsg.ToolCalls,
		}
		intermediateMsgs = append(intermediateMsgs, assistantMsg)
		currentHistory = append(currentHistory, assistantMsg)

		//LLM指示调用MCP工具
		toolResults := s.executeNativeToolCalls(ctx, respMsg.ToolCalls, sender, toolsMap)
		intermediateMsgs = append(intermediateMsgs, toolResults...)
		currentHistory = append(currentHistory, toolResults...)
	}

	global.Log.Warnf("会话 %d 超出了 ReAct 循环最大次数，强制兜底转人工。", param.Conversation.ID)
	return &common.LlmComplexResponse{TransferToHuman: true}, intermediateMsgs, nil
}

// executeNativeToolCalls 原生函数调用的执行与拦截封装
func (s *llmService) executeNativeToolCalls(ctx context.Context, toolCalls []openai.ToolCall, sender common.Sender, toolsMap map[string]mappedTool) []common.LlmMessage {
	var toolResults []common.LlmMessage
	var mu sync.Mutex
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, tc := range toolCalls {
		g.Go(func() error {
			var resultStr string
			if meta, ok := toolsMap[tc.Function.Name]; ok {
				safeArgs, shouldExecute, rejectReason := s.ensureSafeArguments(tc.Function.Arguments, meta.Def, sender)
				if !shouldExecute {
					resultStr = rejectReason
				} else {
					rawResult, err := global.McpService.ExecuteTool(gCtx, meta.ClientName, meta.ToolName, safeArgs)
					if err != nil {
						resultStr = fmt.Errorf("[%s]%w", tc.Function.Name, err).Error()
					} else if !s.verifyToolResultOwnership(rawResult, sender) {
						currentUserID := ""
						if sender.Identifier != nil {
							currentUserID = *sender.Identifier
						}
						resultStr = fmt.Sprintf("安全拦截: 拒绝访问。数据归属与当前用户(%s)不一致。", currentUserID)
					} else {
						resultStr = rawResult
					}
				}
			} else {
				resultStr = fmt.Sprintf("未找到名为 '%s' 的工具", tc.Function.Name)
			}

			// 截断结果
			const maxToolResultLength = 4096
			runes := []rune(resultStr)
			if len(runes) > maxToolResultLength {
				resultStr = string(runes[:maxToolResultLength]) + "\n...(内容过长已截断)"
			}

			global.Log.Debugf("========= MCP 工具 [%s] 原生执行结果: %s", tc.Function.Name, resultStr)

			mu.Lock()
			toolResults = append(toolResults, common.LlmMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    resultStr,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
			})
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait()
	return toolResults
}

// ensureSafeArguments 检查并注入必要的安全参数(如user_id)，返回处理后的参数和是否允许执行
func (s *llmService) ensureSafeArguments(args string, toolDef mcp.Tool, sender common.Sender) (jsontext.Value, bool, string) {
	var argMap map[string]any
	if err := json.Unmarshal([]byte(args), &argMap); err != nil {
		if argMap == nil {
			argMap = make(map[string]any)
		}
	}

	var schemaMap map[string]any
	if b, err := json.Marshal(toolDef.InputSchema); err == nil {
		_ = json.Unmarshal(b, &schemaMap)
	}

	props, ok := schemaMap["properties"].(map[string]any)
	if !ok {
		return jsontext.Value(args), true, ""
	}

	if _, hasUser := props[admin.McpArgUserId]; hasUser {
		var safeUserID string
		if sender.Identifier != nil {
			safeUserID = *sender.Identifier
		}

		if safeUserID == "" {
			global.Log.Warnf("[executeNativeToolCalls] 拦截匿名用户调用敏感工具: %s", toolDef.Name)
			return nil, false, "执行失败: 当前用户未登录(无身份标识)，无法执行涉及用户数据的操作。"
		}

		argMap[admin.McpArgUserId] = safeUserID
		if newArgs, err := json.Marshal(argMap); err == nil {
			return newArgs, true, ""
		}
	}

	return jsontext.Value(args), true, ""
}

// verifyToolResultOwnership 检查工具返回的数据是否属于当前用户
func (s *llmService) verifyToolResultOwnership(rawResult string, sender common.Sender) bool {
	// 尝试解析JSON结果
	var resultData map[string]any
	if json.Unmarshal([]byte(rawResult), &resultData) != nil {
		// 非JSON结果，默认安全(或无法判断归属)，放行
		return true
	}

	// 提取数据中的用户ID
	var dataUserID string
	if v, ok := resultData[admin.McpArgUserId]; ok {
		dataUserID = fmt.Sprintf("%v", v)
	} else if v, ok := resultData["userId"]; ok {
		dataUserID = fmt.Sprintf("%v", v)
	} else if v, ok := resultData["uid"]; ok {
		dataUserID = fmt.Sprintf("%v", v)
	}

	// 如果数据中没有包含用户ID，则认为不涉及敏感归属，放行
	if dataUserID == "" {
		return true
	}

	// 比较当前用户ID
	currentUserID := ""
	if sender.Identifier != nil {
		currentUserID = *sender.Identifier
	}

	return dataUserID == currentUserID
}
