package task

import (
	"context"
	"errors"
	"fmt"
	"os" // 用于获取主机名作为Agent ID
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/internal/vector"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

// 同步Chatwoot关键字到Redis和向量数据库
func (m *Manager) KeywordReloader() error {
	ctx := context.Background()
	global.Log.Println("开始同步Chatwoot关键词...")

	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}

	responses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return fmt.Errorf("从Chatwoot获取预设回复失败: %w", err)
	}

	var (
		exactMatchRules        []chatwoot.CannedResponse
		semanticRulesToProcess []chatwoot.CannedResponse
	)

	semanticPrefix := global.Config.Ai.SemanticPrefix
	hybridPrefix := global.Config.Ai.HybridPrefix

	// 1. 根据规则进行分类
	for _, resp := range responses {
		if resp.Content == "" || resp.ShortCode == "" {
			continue
		}

		if hybridPrefix != "" && strings.HasPrefix(resp.ShortCode, hybridPrefix) {
			// 混合模式: 同时用于语义匹配和精确匹配
			semanticRulesToProcess = append(semanticRulesToProcess, resp)

			// 提取关键字用于精确匹配
			exactMatchRule := resp
			keyword := strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, hybridPrefix))
			if keyword != "" {
				exactMatchRule.ShortCode = strings.ToLower(keyword)
				exactMatchRules = append(exactMatchRules, exactMatchRule)
			}
		} else if strings.HasPrefix(resp.ShortCode, semanticPrefix) {
			// 仅语义匹配
			semanticRulesToProcess = append(semanticRulesToProcess, resp)
		} else {
			// 仅精确匹配
			resp.ShortCode = strings.ToLower(resp.ShortCode)
			exactMatchRules = append(exactMatchRules, resp)
		}
	}

	global.Log.Infof("分类完成: %d 条精确匹配规则, %d 条语义匹配规则待处理", len(exactMatchRules), len(semanticRulesToProcess))

	// --- 语义规则处理 ---
	var documentsForVectorDB []vector.Document
	var allSemanticIDs []string

	if len(semanticRulesToProcess) > 0 {
		// 2. 并发处理所有语义匹配规则（仅LLM生成问题）
		if global.LlmService == nil {
			return fmt.Errorf("LLM客户端未初始化")
		}
		type semanticJob struct {
			resp             chatwoot.CannedResponse
			standardQuestion string
		}

		var (
			llmGroup      errgroup.Group
			mu            sync.Mutex
			completedJobs []semanticJob
		)
		llmGroup.SetLimit(5)

		for _, resp := range semanticRulesToProcess {
			resp := resp // 避免闭包陷阱

			var seedQuestion string
			if hybridPrefix != "" && strings.HasPrefix(resp.ShortCode, hybridPrefix) {
				seedQuestion = strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, hybridPrefix))
			} else {
				seedQuestion = strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, semanticPrefix))
			}

			llmGroup.Go(func() error {
				var llmInputText string
				var promptToUse enum.SystemPrompt

				if seedQuestion != "" {
					llmInputText = seedQuestion
					promptToUse = enum.SystemPromptGenQuestionFromKeyword
				} else {
					llmInputText = resp.Content
					promptToUse = enum.SystemPromptGenQuestionFromContent
				}

				standardQuestion, err := global.LlmService.GenerateStandardQuestion(ctx, promptToUse, llmInputText)
				if err != nil {
					global.Log.Warnf("为ID %d 的内容生成标准问题失败: %v", resp.Id, err)
					return nil
				}
				standardQuestion = strings.Trim(standardQuestion, `"'。， `)
				if standardQuestion == "" {
					global.Log.Warnf("LLM为ID %d 的内容生成了空问题", resp.Id)
					return nil
				}

				if gin.Mode() == gin.DebugMode {
					fmt.Printf("==========LLM生成标准问题: '%s'", llmInputText)
				}

				mu.Lock()
				completedJobs = append(completedJobs, semanticJob{resp: resp, standardQuestion: standardQuestion})
				mu.Unlock()
				return nil
			})
		}

		if err := llmGroup.Wait(); err != nil {
			global.Log.Errorf("处理LLM生成标准问题任务时发生意外错误: %v", err)
		}

		// 3. 批量为所有生成的标准问题创建向量
		if len(completedJobs) > 0 {
			keywordToEmbed := make([]string, 0, len(completedJobs))
			for _, job := range completedJobs {
				// keywordToEmbed = append(keywordToEmbed, fmt.Sprintf("问题: %s; 答案: %s", job.standardQuestion, job.resp.Content))
				keywordToEmbed = append(keywordToEmbed, job.standardQuestion)
			}

			// 批量为所有生成的标准问题创建向量。
			embedCtx, cancel := context.WithTimeout(ctx, time.Duration(global.Config.LlmEmbedding.BatchTimeout)*time.Second)
			defer cancel()

			embeddings, err := global.EmbeddingService.CreateEmbeddings(embedCtx, keywordToEmbed)
			if err != nil {
				global.Log.Errorf("批量创建向量失败: %v", err)
			} else if len(embeddings) == len(completedJobs) {
				// 4. 构建最终的向量文档
				for i, job := range completedJobs {
					docID := fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, job.resp.Id)
					doc := vector.Document{
						ID: docID,
						Metadata: map[string]interface{}{
							dao.VectorMetadataKeyQuestion: job.standardQuestion,
							dao.VectorMetadataKeyAnswer:   job.resp.Content,
							dao.VectorMetadataKeySourceID: int64(job.resp.Id),
						},
						Embedding: embeddings[i],
					}
					documentsForVectorDB = append(documentsForVectorDB, doc)
					allSemanticIDs = append(allSemanticIDs, docID)
				}
			}
		}
	}

	// 5. 并发执行数据同步到Redis和向量数据库
	var finalGroup errgroup.Group
	finalGroup.Go(func() error {
		return syncRedis(ctx, exactMatchRules)
	})
	finalGroup.Go(func() error {
		if err := syncVectorDb(ctx, documentsForVectorDB, allSemanticIDs); err != nil {
			global.Log.Errorf("同步关键词到向量数据库时发生非阻塞错误: %v", err)
		}
		return nil
	})
	if err := finalGroup.Wait(); err != nil {
		return err
	}

	// 6. 重新加载关键词到内存
	return m.LoadKeywords()
}

// syncRedis 处理精确匹配规则到Redis的同步
func syncRedis(ctx context.Context, exactMatchRules []chatwoot.CannedResponse) error {
	if global.RedisClient == nil {
		return errors.New("Redis客户端未初始化")
	}

	// 获取当前Redis中所有的short_code，用于判断哪些需要删除
	existingRedisData, err := global.RedisClient.HGetAll(ctx, redis.KeyCannedResponsesHash).Result()
	if err != nil {
		return fmt.Errorf("获取现有Redis快捷回复失败: %w", err)
	}

	existingShortCodes := make(map[string]struct{})
	for sc := range existingRedisData {
		existingShortCodes[sc] = struct{}{}
	}

	// 构建新的short_code集合
	newShortCodes := make(map[string]struct{})
	for _, rule := range exactMatchRules {
		newShortCodes[rule.ShortCode] = struct{}{}
	}

	// 找出需要从Redis中删除的short_code
	var shortCodesToDelete []string
	for sc := range existingShortCodes {
		if _, exists := newShortCodes[sc]; !exists {
			shortCodesToDelete = append(shortCodesToDelete, sc)
		}
	}

	// 删除旧数据
	if len(shortCodesToDelete) > 0 {
		deletedCount, err := dao.App.KeywordsDb.DeleteKeywordsFromRedis(ctx, shortCodesToDelete)
		if err != nil {
			global.Log.Warnf("从Redis删除 %d 条旧快捷回复失败: %v", len(shortCodesToDelete), err)
		} else {
			global.Log.Infof("从Redis删除 %d 条旧快捷回复", deletedCount)
		}
	}

	// 插入/更新新数据
	savedCount, err := dao.App.KeywordsDb.SaveKeywordsToRedis(ctx, exactMatchRules)
	if err != nil {
		global.Log.Errorln("[isfsifi]同步关键词到Redis失败:", err)
		return fmt.Errorf("同步关键词到Redis失败: %w", err)
	}

	global.Log.Printf("成功从Chatwoot同步 %d 条关键词到Redis", savedCount)
	return nil
}

// syncVectorDb 向量数据库处理
func syncVectorDb(ctx context.Context, documents []vector.Document, activeIDs []string) error {
	if len(documents) > 0 {
		upsertedCount, err := dao.App.VectorDb.BatchUpsert(ctx, documents)
		if err != nil {
			global.Log.Errorln("[gsgf4g]同步关键词到向量数据库失败:", err)
			return fmt.Errorf("同步关键词到向量数据库失败: %w", err)
		}
		global.Log.Printf("成功同步 %d 条语义匹配规则到向量数据库", upsertedCount)
	} else {
		global.Log.Println("没有需要同步到向量数据库的语义匹配规则")
	}

	// 清理向量数据库中不再存在的条目
	prunedCount, err := dao.App.VectorDb.PruneStale(ctx, activeIDs)
	if err != nil {
		global.Log.Warnf("[gjsf8g]清理向量数据库中过期条目失败: %v", err)
	}
	if prunedCount > 0 {
		global.Log.Printf("成功从向量数据库中清理 %d 条过期条目", prunedCount)
	}

	return nil
}

// LoadKeywords 从Redis加载关键词到内存，并处理分布式锁
func (m *Manager) LoadKeywords() error {
	ctx := context.Background()
	var (
		err       error
		responses []chatwoot.CannedResponse
		agentID   string
	)

	// 获取当前主机名作为Agent ID，用于分布式锁
	agentID, err = os.Hostname()
	if err != nil {
		agentID = "unknown_agent"
		global.Log.Warnf("获取主机名失败，使用默认Agent ID: %s", agentID)
	}

	// 尝试从Redis加载数据
	responses, err = dao.App.KeywordsDb.LoadAllKeywordsFromRedis(ctx)
	if err != nil {
		global.Log.Errorf("从Redis加载Keywords失败: %v", err)
		// 如果Redis加载失败，尝试获取锁进行同步
		goto TrySync
	}

	if len(responses) == 0 {
		global.Log.Infoln("Redis中没有快捷回复数据，尝试进行同步...")
		goto TrySync
	}

	// Redis有数据，直接加载到内存
	m.updateInMemoryMap(responses)
	return nil

TrySync:
	// 尝试获取分布式锁
	locked, err := dao.App.KeywordsDb.AcquireSyncLock(ctx, agentID)
	if err != nil {
		global.Log.Errorf("获取Redis同步锁失败: %v", err)
		return fmt.Errorf("获取Redis同步锁失败: %w", err)
	}

	if locked {
		defer func() {
			if releaseErr := dao.App.KeywordsDb.ReleaseSyncLock(ctx, agentID); releaseErr != nil {
				global.Log.Errorf("释放Redis同步锁失败: %v", releaseErr)
			}
		}()

		// 执行全量同步
		if err := m.KeywordReloader(); err != nil {
			global.Log.Errorf("初始同步Chatwoot关键词失败: %v", err)
			return fmt.Errorf("初始同步Chatwoot关键词失败: %w", err)
		}
		global.Log.Infoln("初始同步Chatwoot关键词完成。")
		return nil
	} else {
		// 其他实例正在同步，等待一段时间后再次尝试从Redis加载
		time.Sleep(time.Duration(global.Config.Redis.LockExpiry/2) * time.Second) // 等待锁过期时间的一半
		responses, err = dao.App.KeywordsDb.LoadAllKeywordsFromRedis(ctx)
		if err != nil {
			global.Log.Errorf("等待后从Redis加载Keywords失败: %v", err)
			return fmt.Errorf("等待后从Redis加载Keywords失败: %w", err)
		}
		if len(responses) == 0 {
			global.Log.Warnf("等待后Redis仍无数据，可能同步失败或网络问题。")
			return errors.New("等待后Redis仍无数据")
		}
		m.updateInMemoryMap(responses)
		return nil
	}
}

// updateInMemoryMap 原子性更新内存中的CannedResponses Map
func (m *Manager) updateInMemoryMap(responses []chatwoot.CannedResponse) {
	tempMap := make(map[string]string)
	for _, resp := range responses {
		// 确保当存在重复的 short_code 时，我们只保留最新（或任意一个）
		tempMap[strings.ToLower(resp.ShortCode)] = resp.Content
	}

	global.CannedResponses.Lock()
	global.CannedResponses.Data = tempMap
	global.CannedResponses.Unlock()

	global.Log.Printf("成功加载 %d 条关键词到内存", len(tempMap))
}
