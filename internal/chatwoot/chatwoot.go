package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gitee.com/taoJie_1/chat/model/enum"
	pkgchatwoot "gitee.com/taoJie_1/chat/pkg/chatwoot"
	"github.com/sirupsen/logrus"
)

// TransferToHumanRequest 定义了转人工API的请求体
type TransferToHumanRequest struct {
	Status string `json:"status"` // "open" 表示转为人工处理
	// 还可以增加 AssigneeID 或 TeamID 来指定客服或团队
	// TeamID int `json:"team_id,omitempty"`
}

// Client 是Chatwoot API的客户端
type Client struct {
	BaseURL    string
	AccountID  int
	ApiToken   string
	HttpClient *http.Client
	Logger     *logrus.Logger
}

// NewClient 创建一个新的Chatwoot客户端实例
func NewClient(baseURL string, accountID int, apiToken string, logger *logrus.Logger) pkgchatwoot.Service {
	return &Client{
		BaseURL:   baseURL,
		AccountID: accountID,
		ApiToken:  apiToken,
		HttpClient: &http.Client{
			Timeout: 10 * time.Second, // 设置10秒超时
		},
		Logger: logger,
	}
}

// 获取所有的预设回复
func (c *Client) GetCannedResponses() ([]pkgchatwoot.CannedResponse, error) {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/canned_responses", c.BaseURL, c.AccountID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("api_access_token", c.ApiToken)

	// 发送请求
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API请求返回非200状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	var responses []pkgchatwoot.CannedResponse
	if err := json.Unmarshal(body, &responses); err != nil {
		return nil, fmt.Errorf("解析JSON响应失败: %w", err)
	}

	return responses, nil
}

// 定义了创建私信备注的请求体
type CreatePrivateNoteRequest struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
}

// 在指定的对话中创建一条私信备注
func (c *Client) CreatePrivateNote(conversationID uint, content string) error {

	noteURL := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", c.BaseURL, c.AccountID, conversationID)
	notePayload := CreatePrivateNoteRequest{
		Content:     content,
		MessageType: "private",
		Private:     true,
	}
	noteBody, err := json.Marshal(notePayload)
	if err != nil {
		return fmt.Errorf("序列化私信备注请求体失败: %w", err)
	}

	req, err := http.NewRequest("POST", noteURL, bytes.NewBuffer(noteBody))
	if err != nil {
		return fmt.Errorf("创建私信备注请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.ApiToken)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送私信备注API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("私信备注API请求返回非200状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// 将会话状态切换为 "open", 转接人工客服
func (c *Client) ToggleConversationStatus(conversationID uint) error {
	// `POST /api/v1/conversations/{id}/assign`：将会话分配给一个具体的人工客服团队（Inbox）

	toggleURL := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/toggle_status", c.BaseURL, c.AccountID, conversationID)

	payload := TransferToHumanRequest{
		Status: "open",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化转人工请求体失败: %w", err)
	}

	req, err := http.NewRequest("POST", toggleURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("创建转人工请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.ApiToken)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送转人工API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("转人工API请求返回非200状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ToggleTypingRequest 定义了切换输入状态API的请求体
type ToggleTypingRequest struct {
	TypingStatus string `json:"typing_status"` // "on" or "off"
}

// 切换指定会话的 "输入中..." 状态
func (c *Client) ToggleTypingStatus(conversationID uint, status string) error {
	if status != "on" && status != "off" {
		return fmt.Errorf("无效的输入状态: %s", status)
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/toggle_typing_status", c.BaseURL, c.AccountID, conversationID)
	payload := ToggleTypingRequest{
		TypingStatus: status,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化输入状态请求体失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("创建切换输入状态请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.ApiToken)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送切换输入状态API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("切换输入状态API请求返回非200状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// CreateMessageRequest 定义了创建消息API的请求体
type CreateMessageRequest struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
}

// 在指定对话中创建一条新消息 (通常是回复)
func (c *Client) CreateMessage(conversationID uint, content string) error {
	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages", c.BaseURL, c.AccountID, conversationID)
	payload := CreateMessageRequest{
		Content:     content,
		MessageType: string(enum.MessageTypeOutgoing), // 代表是机器人或客服发出的消息
		Private:     false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化创建消息请求体失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("创建消息请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", c.ApiToken)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送创建消息API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("创建消息API请求返回非200状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}
