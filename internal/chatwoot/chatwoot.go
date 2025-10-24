package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gitee.com/taoJie_1/mall-agent/model/enum"
	"github.com/sirupsen/logrus"
)

type AccountDetails struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type CannedResponse struct {
	Id        int    `json:"id"`
	AccountId int    `json:"account_id"`
	ShortCode string `json:"short_code"`
	Content   string `json:"content"`
}

// ConversationMessagesResponse 定义了会话消息列表API的响应结构
type ConversationMessagesResponse struct {
	Payload []Message `json:"payload"`
}

// Message 结构体定义了Chatwoot API返回的单条消息结构
type Message struct {
	ID          uint         `json:"id"`
	Content     string       `json:"content"`
	MessageType int          `json:"message_type"` // 0: incoming, 1: outgoing
	CreatedAt   int64        `json:"created_at"`
	Sender      Sender       `json:"sender"`
	Private     bool         `json:"private"`     // 是否是私信备注
	Attachments []Attachment `json:"attachments"` // 附件列表
}

// Sender 结构体定义了消息发送者的信息
type Sender struct {
	ID   uint   `json:"id"`
	Type string `json:"type"` // "contact", "agent"
}

// Attachment 结构体定义了消息附件的信息
type Attachment struct {
	ID       uint   `json:"id"`
	FileType string `json:"file_type"`
	FileUrl  string `json:"file_url"`
}

type Service interface {
	// 获取所有的预设回复
	GetCannedResponses() ([]CannedResponse, error)
	//获取用户信息
	GetAccountDetails() (*AccountDetails, error)
	// 在指定的对话中创建一条私信备注
	CreatePrivateNote(conversationID uint, content string) error
	// 将会话状态切换为指定状态
	SetConversationStatus(conversationID uint, status enum.ConversationStatus) error
	// 切换指定会话的 "输入中..." 状态
	ToggleTypingStatus(conversationID uint, status string) error
	// 在指定对话中创建一条新消息 (通常是回复)
	CreateMessage(conversationID uint, content string) error
	// 从Chatwoot API获取指定会话的历史消息
	GetConversationMessages(accountID, conversationID uint) ([]Message, error)
}

// TransferToHumanRequest 定义了转人工API的请求体
type TransferToHumanRequest struct {
	Status enum.ConversationStatus `json:"status"` // "open" 表示转为人工处理
	// 还可以增加 AssigneeID 或 TeamID 来指定客服或团队
	// TeamID int `json:"team_id,omitempty"`
}

// Client 是Chatwoot API的客户端
type Client struct {
	BaseURL       string
	AccountID     int
	AgentApiToken string
	BotApiToken   string
	HttpClient    *http.Client
	Logger        *logrus.Logger
}

// NewClient 创建一个新的Chatwoot客户端实例
func NewClient(baseURL string, accountID int, agentApiToken, botApiToken string, logger *logrus.Logger) Service {
	return &Client{
		BaseURL:       baseURL,
		AccountID:     accountID,
		AgentApiToken: agentApiToken,
		BotApiToken:   botApiToken,
		HttpClient: &http.Client{
			Timeout: 10 * time.Second, // 设置10秒超时
		},
		Logger: logger,
	}
}

type tokenType int

const (
	agentToken tokenType = iota //==0
	botToken                    //==1
)

// sendRequest 是一个通用的请求发送函数，用于处理所有与Chatwoot API的交互
func (c *Client) sendRequest(method, path string, token tokenType, requestBody, responsePayload interface{}) error {
	url := fmt.Sprintf("%s%s", c.BaseURL, path)

	var bodyReader io.Reader
	if requestBody != nil {
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("序列化请求体失败: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token == agentToken {
		req.Header.Set("api_access_token", c.AgentApiToken)
	} else {
		req.Header.Set("api_access_token", c.BotApiToken)
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API请求返回非200状态码: %d, Path: %s, 响应: %s", resp.StatusCode, path, string(bodyBytes))
	}

	if responsePayload != nil {
		if err := json.NewDecoder(resp.Body).Decode(responsePayload); err != nil {
			return fmt.Errorf("解析JSON响应失败: %w", err)
		}
	}

	return nil
}

func (c *Client) GetAccountDetails() (*AccountDetails, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d", c.AccountID)
	var accountDetails AccountDetails
	err := c.sendRequest("GET", path, agentToken, nil, &accountDetails)
	if err != nil {
		return nil, err
	}
	return &accountDetails, nil
}

func (c *Client) GetCannedResponses() ([]CannedResponse, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d/canned_responses", c.AccountID)
	var responses []CannedResponse
	err := c.sendRequest("GET", path, agentToken, nil, &responses)
	if err != nil {
		return nil, err
	}
	return responses, nil
}

// 定义了创建私信备注的请求体
type CreatePrivateNoteRequest struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
}

func (c *Client) CreatePrivateNote(conversationID uint, content string) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.AccountID, conversationID)
	notePayload := CreatePrivateNoteRequest{
		Content:     content,
		MessageType: string(enum.MessageTypeOutgoing),
		Private:     true,
	}
	return c.sendRequest("POST", path, botToken, notePayload, nil)
}

func (c *Client) SetConversationStatus(conversationID uint, status enum.ConversationStatus) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/toggle_status", c.AccountID, conversationID)
	payload := TransferToHumanRequest{
		Status: status,
	}
	return c.sendRequest("POST", path, botToken, payload, nil)
}

// ToggleTypingRequest 定义了切换输入状态API的请求体
type ToggleTypingRequest struct {
	TypingStatus string `json:"typing_status"` // "on" or "off"
}

func (c *Client) ToggleTypingStatus(conversationID uint, status string) error {
	if status != "on" && status != "off" {
		return fmt.Errorf("无效的输入状态: %s", status)
	}
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/toggle_typing_status", c.AccountID, conversationID)
	payload := ToggleTypingRequest{
		TypingStatus: status,
	}
	return c.sendRequest("POST", path, agentToken, payload, nil)
}

// CreateMessageRequest 定义了创建消息API的请求体
type CreateMessageRequest struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
	ContentType string `json:"content_type"`
}

func (c *Client) CreateMessage(conversationID uint, content string) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.AccountID, conversationID)
	payload := CreateMessageRequest{
		Content:     content,
		MessageType: string(enum.MessageTypeOutgoing), // 代表是机器人或客服发出的消息
		Private:     false,
		ContentType: "text",
	}
	return c.sendRequest("POST", path, botToken, payload, nil)
}

func (c *Client) GetConversationMessages(accountID, conversationID uint) ([]Message, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", accountID, conversationID)
	var response ConversationMessagesResponse
	err := c.sendRequest("GET", path, agentToken, nil, &response)
	if err != nil {
		return nil, err
	}
	return response.Payload, nil
}
