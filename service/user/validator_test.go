package user

import (
	"testing"

	"github.com/twbworld/agent/model/common"
)

func TestValidator_ValidatorChatRequest(t *testing.T) {
	v := &validator{}

	tests := []struct {
		name    string
		req     *common.ChatRequest
		wantErr bool
	}{
		{
			name: "验证通过_有纯文本内容",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 1},
				Conversation: common.Conversation{ID: 1},
				Content:      "hello AI",
			},
			wantErr: false,
		},
		{
			name: "验证通过_无文本但包含附件",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 1},
				Conversation: common.Conversation{ID: 1},
				Content:      "",
				Attachments:  []common.Attachment{{ID: 100}},
			},
			wantErr: false,
		},
		{
			name: "验证通过_既有文本又有附件",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 1},
				Conversation: common.Conversation{ID: 1},
				Content:      "请看这张截图",
				Attachments:  []common.Attachment{{ID: 100}},
			},
			wantErr: false,
		},
		{
			name: "验证阻断_Account ID缺失",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 0},
				Conversation: common.Conversation{ID: 1},
				Content:      "hello",
			},
			wantErr: true,
		},
		{
			name: "验证阻断_Conversation ID缺失",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 1},
				Conversation: common.Conversation{ID: 0},
				Content:      "hello",
			},
			wantErr: true,
		},
		{
			name: "验证阻断_内容和附件均为空",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 1},
				Conversation: common.Conversation{ID: 1},
				Content:      "",
				Attachments:  nil,
			},
			wantErr: true,
		},
		{
			name: "验证阻断_内容仅含空白字符且无附件",
			req: &common.ChatRequest{
				Account:      common.Account{ID: 1},
				Conversation: common.Conversation{ID: 1},
				Content:      "   \t\n  ",
				Attachments:  []common.Attachment{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatorChatRequest(tt.req)
			hasErr := err != nil

			if hasErr != tt.wantErr {
				t.Errorf("测试用例 [%s] 期望报错状态为: %v, 但实际得到的 error = %v", tt.name, tt.wantErr, err)
			}
		})
	}
}
