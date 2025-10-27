package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"
	"gitee.com/taoJie_1/mall-agent/internal/redis"
	"gitee.com/taoJie_1/mall-agent/internal/vector"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"golang.org/x/sync/errgroup"
)

// 同步Chatwoot关键字到Redis和向量数据库
func (m *Manager) KeywordReloader() error {
	ctx := context.Background()
	agentID, _ := os.Hostname()
	if agentID == "" {
		agentID = "unknown_agent"
	}

	// --- 获取分布式锁 ---
	locked, err := dao.App.KeywordsDb.AcquireSyncLock(ctx, agentID)
	if err != nil {
		// 如果检查锁时Redis出错，则返回严重错误
		return fmt.Errorf("检查同步锁失败: %w", err)
	}
	if !locked {
		// 未获取到锁，说明另一实例正在同步，本次任务安全退出
		global.Log.Info("同步锁被其他实例持有，跳过本次同步任务")
		return nil
	}
	// 确保函数退出时释放锁
	defer func() {
		if releaseErr := dao.App.KeywordsDb.ReleaseSyncLock(ctx, agentID); releaseErr != nil {
			global.Log.Errorf("释放Redis同步锁失败: %v", releaseErr)
		}
	}()

	global.Log.Info("获取同步锁成功，开始同步Chatwoot关键词...")

	if global.ChatwootService == nil {
		return errors.New("Chatwoot客户端未初始化")
	}

	// 1. 获取上次同步的时间戳
	var lastSyncTime time.Time
	if global.RedisClient == nil {
		global.Log.Warn("Redis客户端未初始化，将执行全量同步且不更新时间戳")
		lastSyncTime = time.Time{}
	} else {
		lastSyncTimeStr, err := global.RedisClient.Get(ctx, redis.KeyLastSyncCannedResponses).Result()
		if err == redis.ErrNil {
			lastSyncTime = time.Time{} // UTC zero time
			global.Log.Info("未找到上次同步时间戳，将执行全量同步")
		} else if err != nil {
			return fmt.Errorf("从Redis获取上次同步时间戳失败: %w", err)
		} else {
			lastSyncTime, err = time.Parse(time.RFC3339Nano, lastSyncTimeStr)
			if err != nil {
				global.Log.Warnf("解析上次同步时间戳 '%s' 失败: %v，将执行全量同步", lastSyncTimeStr, err)
				lastSyncTime = time.Time{}
			}
		}
	}

	// 2. 从Chatwoot拉取全量数据
	responses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return fmt.Errorf("从Chatwoot获取预设回复失败: %w", err)
	}

	// 3. 分类、过滤并确定需要处理的条目
	newLatestSyncTime := lastSyncTime
	var (
		exactMatchRules        []chatwoot.CannedResponse // 用于精确匹配，始终是全量
		semanticRulesToProcess []chatwoot.CannedResponse // 用于语义处理，仅包含增量
		allSemanticIDs         []string                  // 用于向量库清理，始终是全量
	)

	semanticPrefix := global.Config.Ai.SemanticPrefix
	hybridPrefix := global.Config.Ai.HybridPrefix

	for _, resp := range responses {
		if resp.Content == "" || resp.ShortCode == "" {
			continue
		}
		// 解析更新时间，并与上次同步时间比较
		updatedAt, err := time.Parse(time.RFC3339Nano, resp.UpdatedAt)
		if err != nil {
			global.Log.Warnf("解析快捷回复(ID: %d)的updated_at '%s' 失败: %v, 将默认处理该条目", resp.Id, resp.UpdatedAt, err)
			updatedAt = time.Now().In(global.Tz)
		}
		if updatedAt.After(newLatestSyncTime) {
			newLatestSyncTime = updatedAt
		}
		needsProcessing := updatedAt.After(lastSyncTime)

		// 根据short_code规则分类
		if hybridPrefix != "" && strings.HasPrefix(resp.ShortCode, hybridPrefix) {
			// ----- 混合模式 -----
			docID := fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, resp.Id)
			allSemanticIDs = append(allSemanticIDs, docID)
			if needsProcessing {
				semanticRulesToProcess = append(semanticRulesToProcess, resp)
			}
			// 提取关键字用于精确匹配 (全量)
			keyword := strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, hybridPrefix))
			if keyword != "" {
				exactMatchRule := resp
				exactMatchRule.ShortCode = strings.ToLower(keyword)
				exactMatchRules = append(exactMatchRules, exactMatchRule)
			}
		} else if strings.HasPrefix(resp.ShortCode, semanticPrefix) {
			// ----- 仅语义匹配 -----
			docID := fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, resp.Id)
			allSemanticIDs = append(allSemanticIDs, docID)
			if needsProcessing {
				semanticRulesToProcess = append(semanticRulesToProcess, resp)
			}
		} else {
			// ----- 仅精确匹配 -----
			exactMatchRule := resp
			exactMatchRule.ShortCode = strings.ToLower(resp.ShortCode)
			exactMatchRules = append(exactMatchRules, exactMatchRule)
		}
	}

	global.Log.Infof("分类完成: 全量精确匹配规则 %d 条，需增量处理的语义规则 %d 条", len(exactMatchRules), len(semanticRulesToProcess))

	// 4. 对增量语义规则进行LLM和向量化处理
	var documentsForVectorDB []vector.Document
	if len(semanticRulesToProcess) > 0 {
		documentsForVectorDB, err = m.processSemanticRules(ctx, semanticRulesToProcess)
		if err != nil {
			return err
		}
	}

	// 5. 并发执行数据同步到Redis和向量数据库
	var redisErr, vectorErr error
	var wg errgroup.Group

	wg.Go(func() error {
		redisErr = syncRedis(ctx, exactMatchRules) // 使用全量精确匹配规则更新Redis
		return redisErr
	})
	wg.Go(func() error {
		// 使用增量数据更新向量库，并使用全量ID清理陈旧数据
		vectorErr = syncVectorDb(ctx, documentsForVectorDB, allSemanticIDs)
		if vectorErr != nil {
			global.Log.Errorf("同步关键词到向量数据库时发生错误: %v", vectorErr)
		}
		return nil // 设计为非阻塞，不中断主流程
	})

	if err := wg.Wait(); err != nil {
		return err // 返回来自syncRedis的致命错误
	}

	// 6. 更新内存缓存
	m.updateInMemoryMap(exactMatchRules)

	// 7. 仅在所有环节都成功时，才更新同步时间戳
	if redisErr == nil && vectorErr == nil {
		if global.RedisClient != nil && newLatestSyncTime.After(lastSyncTime) {
			if err := global.RedisClient.Set(ctx, redis.KeyLastSyncCannedResponses, newLatestSyncTime.Format(time.RFC3339Nano), 0).Err(); err != nil {
				global.Log.Errorf("更新同步时间戳失败: %v", err)
			} else {
				global.Log.Infof("同步时间戳已更新为: %s", newLatestSyncTime.Format(time.RFC3339Nano))
			}
		}
	} else {
		global.Log.Warn("由于同步过程中发生错误，本次将不更新同步时间戳，以便下次重试")
	}

	global.Log.Infof("Chatwoot关键词同步任务完成")
	return nil
}

// processSemanticRules 是一个辅助函数，用于处理需要LLM调用和向量化的语义规则
func (m *Manager) processSemanticRules(ctx context.Context, rules []chatwoot.CannedResponse) ([]vector.Document, error) {
	if global.LlmService == nil || m.embeddingService == nil {
		return nil, errors.New("LLM或Embedding服务未初始化")
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
	llmGroup.SetLimit(10) // 增加并发限制以加快处理速度

	semanticPrefix := global.Config.Ai.SemanticPrefix
	hybridPrefix := global.Config.Ai.HybridPrefix

	for _, resp := range rules {
		resp := resp // 避免闭包陷阱

		llmGroup.Go(func() error {
			var seedQuestion string
			if hybridPrefix != "" && strings.HasPrefix(resp.ShortCode, hybridPrefix) {
				seedQuestion = strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, hybridPrefix))
			} else {
				seedQuestion = strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, semanticPrefix))
			}

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
				return nil // 不中断其他任务
			}
			standardQuestion = strings.Trim(standardQuestion, `"'。， `)
			if standardQuestion == "" {
				global.Log.Warnf("LLM为ID %d 的内容生成了空问题", resp.Id)
				return nil
			}

			global.Log.Debugf("LLM为种子 '%s' 生成标准问题: '%s'", llmInputText, standardQuestion)

			mu.Lock()
			completedJobs = append(completedJobs, semanticJob{resp: resp, standardQuestion: standardQuestion})
			mu.Unlock()
			return nil
		})
	}

	if err := llmGroup.Wait(); err != nil {
		global.Log.Errorf("处理LLM生成标准问题任务时发生意外错误: %v", err)
	}

	if len(completedJobs) == 0 {
		return nil, nil
	}

	// 批量为所有生成的标准问题创建向量(其实也可以在上一步的LLM生成向量, 但向量质量不如Embedding模型)
	questionsToEmbed := make([]string, len(completedJobs))
	for i, job := range completedJobs {
		questionsToEmbed[i] = job.standardQuestion
	}

	embedCtx, cancel := context.WithTimeout(ctx, time.Duration(global.Config.LlmEmbedding.BatchTimeout)*time.Second)
	defer cancel()

	embeddings, err := m.embeddingService.CreateEmbeddings(embedCtx, questionsToEmbed)
	if err != nil {
		return nil, fmt.Errorf("批量创建向量失败: %w", err)
	}
	if len(embeddings) != len(completedJobs) {
		return nil, fmt.Errorf("向量化返回结果数量 %d 与预期 %d 不匹配", len(embeddings), len(completedJobs))
	}

	// 构建最终的向量文档
	var documentsForVectorDB []vector.Document
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
	}

	return documentsForVectorDB, nil
}

// LoadKeywords 从Redis加载关键词到内存，并处理分布式锁(移除goto)
func (m *Manager) LoadKeywords() error {
	ctx := context.Background()

	// 1. 尝试从Redis加载数据
	responses, err := dao.App.KeywordsDb.LoadAllKeywordsFromRedis(ctx)
	if err == nil && len(responses) > 0 {
		// 缓存命中，直接加载到内存
		m.updateInMemoryMap(responses)
		global.Log.Info("从Redis成功加载快捷回复到内存")
		return nil
	}
	if err != nil {
		global.Log.Errorf("从Redis加载Keywords失败: %v", err)
	} else {
		global.Log.Infoln("Redis中没有快捷回复数据，将触发首次同步")
	}

	// 2. 缓存未命中或加载失败，执行一次同步
	// KeywordReloader内部会处理分布式锁
	if err := m.KeywordReloader(); err != nil {
		return fmt.Errorf("启动时同步关键词失败: %w", err)
	}

	// 3. 检查同步后内存中是否已有数据
	global.CannedResponses.RLock()
	mapLen := len(global.CannedResponses.Data)
	global.CannedResponses.RUnlock()

	if mapLen == 0 {
		// 如果执行同步后内存依然为空，可能意味着同步被跳过（其他实例持有锁）但最终数据仍未加载成功。
		// 这是一个潜在的启动问题，因此返回错误。
		return errors.New("执行同步后，内存中的快捷回复数据仍为空")
	}

	return nil
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
	if len(exactMatchRules) > 0 {
		savedCount, err := dao.App.KeywordsDb.SaveKeywordsToRedis(ctx, exactMatchRules)
		if err != nil {
			global.Log.Errorln("[isfsifi]同步关键词到Redis失败:", err)
			return fmt.Errorf("同步关键词到Redis失败: %w", err)
		}
		global.Log.Printf("成功从Chatwoot同步 %d 条关键词到Redis", savedCount)
	}

	return nil
}

// syncVectorDb 向量数据库处理
func syncVectorDb(ctx context.Context, documents []vector.Document, activeIDs []string) error {
	if global.VectorDb == nil {
		return errors.New("向量数据库客户端未初始化")
	}

	if len(documents) > 0 {
		upsertedCount, err := dao.App.VectorDb.BatchUpsert(ctx, documents)
		if err != nil {
			return fmt.Errorf("同步关键词到向量数据库失败: %w", err)
		}
		global.Log.Printf("成功同步 %d 条语义匹配规则到向量数据库", upsertedCount)
	} else {
		global.Log.Println("没有需要增量同步到向量数据库的语义匹配规则")
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
