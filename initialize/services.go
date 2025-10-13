package initialize

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/chatwoot"
	"gitee.com/taoJie_1/chat/internal/embedding"
	"gitee.com/taoJie_1/chat/internal/llm"
	"gitee.com/taoJie_1/chat/internal/vector"
	"gitee.com/taoJie_1/chat/model/config"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/sashabaranov/go-openai"
)

func (i *Initializer) initVectorDb() error {
	client, err := vector.NewClient(
		global.Config.VectorDb.Url,
		global.Config.VectorDb.Auth,
	)
	if err != nil {
		global.Log.Warnf("创建VectorDb客户端失败: %v", err)
		return err
	}

	// 通过心跳检测验证与VectorDb服务的连接
	err = client.Heartbeat(context.Background())
	if err != nil {
		global.Log.Warnf("无法连接到VectorDb服务 (url: %s): %v", global.Config.VectorDb.Url, err)
		return err
	}

	global.VectorDb = client
	dao.App.VectorDb.CollectionName = global.Config.VectorDb.CollectionName
	global.Log.Info("初始化VectorDb服务成功")
	return nil
}

// vectorDbClose 关闭VectorDb客户端连接
func (i *Initializer) vectorDbClose() error {
	if global.VectorDb != nil {
		return global.VectorDb.Close()
	}
	return nil
}

func (i *Initializer) initChatwoot() error {
	client := chatwoot.NewClient(
		global.Config.Chatwoot.Url,
		int(global.Config.Chatwoot.AccountId),
		global.Config.Chatwoot.Auth,
		global.Log,
	)

	// 通过尝试获取预设回复来验证与Chatwoot服务的连接和认证
	if _, err := client.GetCannedResponses(); err != nil {
		return fmt.Errorf("无法连接到Chatwoot服务 (url: %s): %w", global.Config.Chatwoot.Url, err)
	}

	global.ChatwootService = client
	global.Log.Info("初始化Chatwoot服务成功")
	return nil
}

func (i *Initializer) initLlm() error {
	if err := i.doInitLlm(); err != nil {
		global.Log.Warnf("初始化LLM服务失败: %v", err)
		return err
	}
	global.Log.Info("初始化LLM服务成功")
	return nil
}

func (i *Initializer) doInitLlm() error {
	if len(global.Config.Llm) == 0 {
		return fmt.Errorf("未配置任何LLM")
	}

	llmClients := make(map[enum.LlmSize]*openai.Client, len(global.Config.Llm))
	for _, cfg := range global.Config.Llm {
		config := openai.DefaultConfig(cfg.Auth)
		config.BaseURL = cfg.Url
		config.HTTPClient = &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second}
		llmClients[enum.LlmSize(cfg.Size)] = openai.NewClientWithConfig(config)
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	// 并发地对所有配置的LLM服务进行连接测试
	wg.Add(len(global.Config.Llm))
	for _, cfg := range global.Config.Llm {
		go func(c config.Llm) {
			defer wg.Done()
			size := enum.LlmSize(c.Size)
			client := llmClients[size]

			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// 通过ListModels接口验证服务是否可用
			if _, err := client.ListModels(reqCtx); err != nil {
				mu.Lock()
				err_msg := fmt.Errorf("无法连接到LLM服务 (size: %s, url: %s): %w", size, c.Url, err)
				errs = append(errs, err_msg)
				mu.Unlock()
			}
		}(cfg)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	global.LlmService = llm.NewClient(
		global.Log,
		llmClients,
		global.Config.Llm,
	)
	return nil
}

func (i *Initializer) initLlmEmbedding() error {
	if err := i.doInitLlmEmbedding(); err != nil {
		global.Log.Warnf("初始化向量化服务失败: %v", err)
		return err
	}
	global.Log.Info("初始化向量化服务成功")
	return nil
}

func (i *Initializer) doInitLlmEmbedding() error {
	config := openai.DefaultConfig(global.Config.LlmEmbedding.Auth)
	config.BaseURL = global.Config.LlmEmbedding.Url
	config.HTTPClient = &http.Client{Timeout: time.Duration(global.Config.LlmEmbedding.Timeout) * time.Second}
	openAIClient := openai.NewClientWithConfig(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 通过ListModels接口验证向量化服务是否可用
	if _, err := openAIClient.ListModels(ctx); err != nil {
		return fmt.Errorf("无法连接到向量化服务 (url: %s): %w", config.BaseURL, err)
	}

	global.EmbeddingService = embedding.NewClient(
		openAIClient,
		global.Config.LlmEmbedding.Model,
	)
	return nil
}
