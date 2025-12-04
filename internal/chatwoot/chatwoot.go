package chatwoot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

type ChatwootWebhook string

const (
	// 来自联系人的新消息。
	MessageTypeIncoming ChatwootWebhook = "incoming"
	// 从应用程序发送的消息。
	MessageTypeOutgoing ChatwootWebhook = "outgoing"
	// 对话中的最终用户。
	SenderTypeContact ChatwootWebhook = "contact"
)

type SenderType string

const (
	// SenderContact 代表消息发送者是联系人/终端用户
	SenderContact SenderType = "contact"
	// SenderUser 代表消息发送者是人工客服
	SenderUser SenderType = "user"
	// SenderAgentBot 代表消息发送者是机器人
	SenderAgentBot SenderType = "agent_bot"
)

type ConversationStatus string

const (
	//会话开放状态, 人工客服可恢复
	ConversationStatusOpen ConversationStatus = "open"
	//会话待处理
	ConversationStatusPending ConversationStatus = "pending"
	//会话已解决
	ConversationStatusResolved ConversationStatus = "resolved"
	//会话暂停
	ConversationStatusSnoozed ConversationStatus = "snoozed"
)

type ChatwootEvent string

const (
	// 用户打开小部件,即点击浮窗(机器人+集成 都会Webhooks)
	EventWebwidgetTriggered ChatwootEvent = "webwidget_triggered"
	// 来自联系人的新消息。(机器人和集成 都会Webhooks)
	EventMessageCreated ChatwootEvent = "message_created"
	// 消息已更新。
	EventMessageUpdated ChatwootEvent = "message_updated"
	// 新对话创建。
	EventConversationCreated ChatwootEvent = "conversation_created"
	// 对话状态更改。
	EventConversationStatusChanged ChatwootEvent = "conversation_status_changed"
	// 对话已更新。
	EventConversationUpdated ChatwootEvent = "conversation_updated"
	// 对话已解决。
	EventConversationResolved ChatwootEvent = "conversation_resolved"
)

type MessageDirection int

const (
	// MessageDirectionIncoming 表示接收的消息
	MessageDirectionIncoming MessageDirection = 0
	// MessageDirectionOutgoing 表示发送的消息
	MessageDirectionOutgoing MessageDirection = 1
)

type ContentType string

const (
	// ContentTypeText 表示文本类型消息
	ContentTypeText ContentType = "text"
	// ContentTypeCards 表示卡片类型消息
	ContentTypeCards ContentType = "cards"
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
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ConversationMessagesResponse 定义了会话消息列表API的响应结构
type ConversationMessagesResponse struct {
	Payload []Message `json:"payload"`
}

// CreateMessageRequest 定义了创建消息API的请求体
type CreateMessageRequest struct {
	Content           string          `json:"content,omitempty"`
	MessageType       ChatwootWebhook `json:"message_type"`
	Private           bool            `json:"private"`
	ContentType       ContentType     `json:"content_type"`
	ContentAttributes interface{}     `json:"content_attributes,omitempty"`
}

// CardAction 定义了卡片中的动作按钮
type CardAction struct {
	Type string `json:"type"` // e.g., "link", "buy"
	Text string `json:"text"`
	URI  string `json:"uri"`
}

// CardItem 定义了单个卡片的内容
type CardItem struct {
	MediaURL    string       `json:"media_url,omitempty"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Actions     []CardAction `json:"actions,omitempty"`
}

// CardContentAttributes 定义了卡片消息的 content_attributes 结构
type CardContentAttributes struct {
	Items []CardItem `json:"items"`
}

// 定义了创建私信备注的请求体
type CreatePrivateNoteRequest struct {
	Content     string          `json:"content"`
	MessageType ChatwootWebhook `json:"message_type"`
	Private     bool            `json:"private"`
}

// ToggleTypingRequest 定义了切换输入状态API的请求体
type ToggleTypingRequest struct {
	TypingStatus string `json:"typing_status"` // "on" or "off"
}

// Message 结构体定义了Chatwoot API返回的单条消息结构
type Message struct {
	ID          uint             `json:"id"`
	Content     string           `json:"content"`
	MessageType MessageDirection `json:"message_type"`
	CreatedAt   int64            `json:"created_at"`
	Sender      struct {
		ID   uint       `json:"id"`
		Type SenderType `json:"type"`
	} `json:"sender"`
	Private     bool         `json:"private"`     // 是否是私信备注
	Attachments []Attachment `json:"attachments"` // 附件列表
}

// Attachment 结构体定义了消息附件的信息
type Attachment struct {
	ID       uint   `json:"id"`
	FileType string `json:"file_type"`
	FileUrl  string `json:"file_url"`
}

// ConversationSummary 定义会话摘要信息
type ConversationSummary struct {
	ID      uint               `json:"id"`
	Status  ConversationStatus `json:"status"`
	InboxID int                `json:"inbox_id"`
}

// ContactConversationsResponse 定义获取联系人会话列表的响应结构
type ContactConversationsResponse struct {
	Payload []ConversationSummary `json:"payload"`
}

// CreateConversationRequest 定义创建会话的请求体
type CreateConversationRequest struct {
	SourceID string `json:"source_id"`
}

// CreateConversationResponse 定义创建会话的响应
type CreateConversationResponse struct {
	ID        uint `json:"id"`
	AccountID uint `json:"account_id"`
}

type Service interface {
	// 获取所有的预设回复
	GetCannedResponses() ([]CannedResponse, error)
	// 创建一个新的预设回复
	CreateCannedResponse(shortCode, content string) (*CannedResponse, error)
	// 更新一个已存在的预设回复
	UpdateCannedResponse(id int, shortCode, content string) (*CannedResponse, error)
	// 删除一个预设回复
	DeleteCannedResponse(id int) error
	//获取用户信息
	GetAccountDetails() (*AccountDetails, error)
	// 在指定的对话中创建一条私信备注
	CreatePrivateNote(conversationID uint, content string) error
	// 主动创建一个新会话
	CreateConversation(sourceID string) (uint, error)
	// 将会话状态切换为指定状态
	SetConversationStatus(conversationID uint, status ConversationStatus) error
	// 切换指定会话的 "输入中..." 状态
	ToggleTypingStatus(conversationID uint, status string) error
	// 在指定对话中创建一条新消息 (通常是回复)
	CreateMessage(conversationID uint, content string) error
	// 在指定对话中创建一条卡片消息
	CreateCardMessage(conversationID uint, content string, cardItems []CardItem) error
	// 从Chatwoot API获取指定会话的历史消息
	GetConversationMessages(accountID, conversationID uint) ([]Message, error)
	// 获取指定联系人的所有会话
	GetContactConversations(contactID uint) ([]ConversationSummary, error)
}

// TransferToHumanRequest 定义了转人工API的请求体
type TransferToHumanRequest struct {
	Status ConversationStatus `json:"status"` // "open" 表示转为人工处理
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

func (c *Client) CreateCannedResponse(shortCode, content string) (*CannedResponse, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d/canned_responses", c.AccountID)
	payload := map[string]string{
		"short_code": shortCode,
		"content":    content,
	}
	var response CannedResponse
	err := c.sendRequest("POST", path, agentToken, payload, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) UpdateCannedResponse(id int, shortCode, content string) (*CannedResponse, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d/canned_responses/%d", c.AccountID, id)
	payload := map[string]string{
		"short_code": shortCode,
		"content":    content,
	}
	var response CannedResponse
	err := c.sendRequest("PATCH", path, agentToken, payload, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DeleteCannedResponse(id int) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/canned_responses/%d", c.AccountID, id)
	return c.sendRequest("DELETE", path, agentToken, nil, nil)
}

func (c *Client) CreatePrivateNote(conversationID uint, content string) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.AccountID, conversationID)
	notePayload := CreatePrivateNoteRequest{
		Content:     content,
		MessageType: MessageTypeOutgoing,
		Private:     true,
	}
	return c.sendRequest("POST", path, botToken, notePayload, nil)
}

func (c *Client) CreateConversation(sourceID string) (uint, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations", c.AccountID)
	payload := CreateConversationRequest{
		SourceID: sourceID,
	}

	var response CreateConversationResponse
	err := c.sendRequest("POST", path, agentToken, payload, &response)
	if err != nil {
		return 0, err
	}
	return response.ID, nil
}

func (c *Client) SetConversationStatus(conversationID uint, status ConversationStatus) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/toggle_status", c.AccountID, conversationID)
	payload := TransferToHumanRequest{
		Status: status,
	}
	return c.sendRequest("POST", path, botToken, payload, nil)
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

func (c *Client) CreateMessage(conversationID uint, content string) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.AccountID, conversationID)
	payload := CreateMessageRequest{
		Content:     content,
		MessageType: MessageTypeOutgoing, // 代表是机器人或客服发出的消息
		Private:     false,
		ContentType: ContentTypeText,
	}
	return c.sendRequest("POST", path, botToken, payload, nil)
}

// content 给客服看的文本
// cardItems 给客户看的卡片
func (c *Client) CreateCardMessage(conversationID uint, content string, cardItems []CardItem) error {
	path := fmt.Sprintf("/api/v1/accounts/%d/conversations/%d/messages", c.AccountID, conversationID)
	payload := CreateMessageRequest{
		Content:     content,
		MessageType: MessageTypeOutgoing,
		Private:     false,
		ContentType: ContentTypeCards,
		ContentAttributes: CardContentAttributes{
			Items: cardItems,
		},
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

func (c *Client) GetContactConversations(contactID uint) ([]ConversationSummary, error) {
	path := fmt.Sprintf("/api/v1/accounts/%d/contacts/%d/conversations", c.AccountID, contactID)
	var response ContactConversationsResponse
	err := c.sendRequest("GET", path, agentToken, nil, &response)
	if err != nil {
		return nil, err
	}
	return response.Payload, nil
}
