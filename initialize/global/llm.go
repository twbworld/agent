package global

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/llm"
	"gitee.com/taoJie_1/chat/model/config"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/sashabaranov/go-openai"
)

// 初始化LLM服务
func initLlm() error {
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

	// 并发健康检查,并收集所有错误
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	wg.Add(len(global.Config.Llm))
	for _, cfg := range global.Config.Llm {
		go func(c config.Llm) {
			defer wg.Done()
			size := enum.LlmSize(c.Size)
			client := llmClients[size]

			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if _, err := client.ListModels(reqCtx); err != nil {
				mu.Lock()
				err_msg := fmt.Errorf("无法连接到LLM服务 (size: %s, url: %s): %w", size, c.Url, err)
				errs = append(errs, err_msg)
				mu.Unlock()
			}
		}(cfg)
	}
	wg.Wait()

	// 如果有任何错误,通过errors.Join合并后返回
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
