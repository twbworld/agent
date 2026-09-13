package user

import (
	"reflect"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/twbworld/agent/model/common"
)

func TestChatApi_trimHistory(t *testing.T) {
	api := &ChatApi{}

	msgU1 := common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: "问题1"}
	msgA1 := common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: "回答1"}
	msgU2 := common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: "问题2"}
	msgA2 := common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: "工具调用", ToolCalls: []openai.ToolCall{{ID: "call_1"}}}
	msgT1 := common.LlmMessage{Role: openai.ChatMessageRoleTool, Content: "工具结果", ToolCallID: "call_1"}
	msgU3 := common.LlmMessage{Role: openai.ChatMessageRoleUser, Content: "问题3"}
	msgA3 := common.LlmMessage{Role: openai.ChatMessageRoleAssistant, Content: "回答3"}

	tests := []struct {
		name                 string
		history              []common.LlmMessage
		maxRounds            int
		maxAssistantPerRound int // 在最新逻辑中未使用，仅占位
		want                 []common.LlmMessage
	}{
		{
			name:      "空历史记录",
			history:   []common.LlmMessage{},
			maxRounds: 3,
			want:      []common.LlmMessage{},
		},
		{
			name:      "限制轮数为0",
			history:   []common.LlmMessage{msgU1, msgA1},
			maxRounds: 0,
			want:      []common.LlmMessage{},
		},
		{
			name:      "保留最后1轮(包含工具调用)",
			history:   []common.LlmMessage{msgU1, msgA1, msgU2, msgA2, msgT1},
			maxRounds: 1,
			want:      []common.LlmMessage{msgU2, msgA2, msgT1}, // 逆序查找，遇到第1个User就截止
		},
		{
			name:      "保留最后2轮",
			history:   []common.LlmMessage{msgU1, msgA1, msgU2, msgA2, msgT1, msgU3, msgA3},
			maxRounds: 2,
			want:      []common.LlmMessage{msgU2, msgA2, msgT1, msgU3, msgA3},
		},
		{
			name:      "最大轮数超过实际轮数",
			history:   []common.LlmMessage{msgU1, msgA1},
			maxRounds: 5,
			want:      []common.LlmMessage{msgU1, msgA1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := api.trimHistory(tt.history, tt.maxRounds, tt.maxAssistantPerRound)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("trimHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}
