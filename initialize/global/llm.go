package global

import (
	"context"
	"net/http"
	"time"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/sashabaranov/go-openai"
)

// 初始化LLM客户端
func (*GlobalInit) initLlm() error {
	if len(global.Config.Llm) == 0 {
		global.Log.Warnln("LLM缺少配置[rejnk33], 相关功能将不可用")
		return nil
	}

	validClients := make(map[enum.LlmSize]*openai.Client)

	var (
		client       *openai.Client
		clientConfig openai.ClientConfig
	)
	for _, cfg := range global.Config.Llm {
		if cfg.Url == "" || cfg.Model == "" {
			global.Log.Warnf("LLM配置项 %s 信息不完整，已跳过", cfg.Size)
			continue
		}

		clientConfig = openai.DefaultConfig(cfg.Auth)
		clientConfig.BaseURL = cfg.Url
		clientConfig.HTTPClient = &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		}
		client = openai.NewClientWithConfig(clientConfig)

		// 连接测试
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
		_, err := client.ListModels(ctx)
		cancel()
		if err != nil {
			global.Log.Warnf("LLM客户端 [%s] 连接测试失败: %v", cfg.Size, err)
			continue
		}

		global.Log.Infof("LLM客户端 [%s] 连接成功", cfg.Size)
		modelSize := enum.LlmSize(cfg.Size)
		validClients[modelSize] = client
	}

	if len(validClients) == 0 {
		global.Log.Warnln("所有LLM配置均无效, 初始化失败, LLM相关功能将不可用")
		return nil
	}

	// 定义模型大小的优先级顺序，用于“降级”查找
	sizes := []enum.LlmSize{enum.ModelSmall, enum.ModelMedium, enum.ModelLarge}

	var lastGoodClient *openai.Client
	for _, size := range sizes {
		if client, ok := validClients[size]; ok {
			global.Llm[size] = client
			lastGoodClient = client
		} else if lastGoodClient != nil {
			// 如果当前大小的模型客户端不存在，但之前有过好的客户端，则使用它作为降级
			global.Llm[size] = lastGoodClient
			global.Log.Warnf("LLM客户端 [%s] 未配置或无效，已降级配置", size)
		}
	}

	if lastGoodClient == nil {
		global.Log.Warnln("未能找到任何可用的LLM客户端, LLM相关功能将不可用")
		return nil
	}
	for _, size := range sizes {
		if global.Llm[size] == nil {
			global.Llm[size] = lastGoodClient
		}
	}

	global.Log.Infoln("LLM服务初始化完成")
	return nil
}

// 初始化向量化服务
func (*GlobalInit) initLlmEmbedding() error {
	if global.Config.LlmEmbedding.Url == "" {
		global.Log.Warnln("向量化模型URL(llm_embedding.url)未配置, RAG功能将不可用")
		return nil
	}

	clientConfig := openai.DefaultConfig(global.Config.LlmEmbedding.Auth)
	clientConfig.BaseURL = global.Config.LlmEmbedding.Url
	clientConfig.HTTPClient = &http.Client{
		Timeout: time.Duration(global.Config.LlmEmbedding.Timeout) * time.Second,
	}
	global.LlmEmbedding = openai.NewClientWithConfig(clientConfig)

	// 连接测试
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(global.Config.LlmEmbedding.Timeout)*time.Second)
	defer cancel()
	if _, err := global.LlmEmbedding.ListModels(ctx); err != nil {
		global.Log.Warnf("向量化服务连接测试失败: %v. RAG功能将不可用", err)
		global.LlmEmbedding = nil
		return nil
	}

	global.Log.Infoln("向量化服务连接成功")
	return nil
}
