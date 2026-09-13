package user

import (
	"reflect"
	"testing"

	"github.com/twbworld/agent/global"
	"github.com/twbworld/agent/model/common"
	"github.com/twbworld/agent/model/config"
)

func TestActionService_UpdateTransferKeywords(t *testing.T) {
	// 初始化全局配置以防空指针
	global.Config = &config.Config{
		Ai: config.Ai{
			TransferKeywords: []string{"转人工", "help"},
		},
	}

	svc := NewActionService().(*actionService)

	// 测试转为小写及集合构建是否正确
	svc.UpdateTransferKeywords([]string{"人工", "HUMAN", "客服"})

	expected := map[string]struct{}{
		"人工":    {},
		"human": {},
		"客服":    {},
	}

	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if !reflect.DeepEqual(svc.transferKeywords, expected) {
		t.Errorf("UpdateTransferKeywords() 失败, 期望 %v, 得到 %v", expected, svc.transferKeywords)
	}
}

func TestActionService_MatchCannedResponse(t *testing.T) {
	// 初始化全局知识库缓存
	global.CannedResponses = &global.CannedResponsesMap{
		Data: map[string]string{
			"你好":    "你好，我是智能客服。",
			"订单查不到": "请提供订单号为您查询。",
		},
	}

	// 初始化全局配置并注入服务
	global.Config = &config.Config{
		Ai: config.Ai{
			TransferKeywords: []string{"转人工", "投诉"},
		},
	}
	svc := NewActionService()

	tests := []struct {
		name       string
		reqContent string
		wantAnswer string
		wantAction bool
		wantErr    bool
	}{
		{
			name:       "空消息过滤",
			reqContent: "   ",
			wantAnswer: "",
			wantAction: false,
			wantErr:    false,
		},
		{
			name:       "匹配到转人工关键词",
			reqContent: "转人工",
			wantAnswer: "",
			wantAction: true, // 触发Action
			wantErr:    false,
		},
		{
			name:       "匹配到转人工关键词(忽略大小写与空格)",
			reqContent: " 投诉 ",
			wantAnswer: "",
			wantAction: true,
			wantErr:    false,
		},
		{
			name:       "匹配到精确快捷回复",
			reqContent: "你好",
			wantAnswer: "你好，我是智能客服。",
			wantAction: false,
			wantErr:    false,
		},
		{
			name:       "未匹配到任何内容",
			reqContent: "今天天气怎么样",
			wantAnswer: "",
			wantAction: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &common.ChatRequest{
				Content: tt.reqContent,
			}
			gotAnswer, gotAction, err := svc.MatchCannedResponse(req)

			if (err != nil) != tt.wantErr {
				t.Errorf("MatchCannedResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAnswer != tt.wantAnswer {
				t.Errorf("MatchCannedResponse() gotAnswer = %v, want %v", gotAnswer, tt.wantAnswer)
			}
			if gotAction != tt.wantAction {
				t.Errorf("MatchCannedResponse() gotAction = %v, want %v", gotAction, tt.wantAction)
			}
		})
	}
}
