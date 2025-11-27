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

// KeywordReloader 作为总同步/审计任务，从 Chatwoot 拉取全量数据，
// 并与上次同步时间对比，找出增量数据进行处理，同时清理已不存在的旧数据。
func (m *Manager) KeywordReloader() error {
	ctx := context.Background()
	agentID, _ := os.Hostname()
	if agentID == "" {
		agentID = "unknown_agent"
	}

	// --- 获取分布式锁 ---
	locked, err := dao.App.KeywordsDb.AcquireSyncLock(ctx, agentID)
	if err != nil {
		return fmt.Errorf("检查同步锁失败: %w", err)
	}
	if !locked {
		global.Log.Info("同步锁被其他实例持有，跳过本次同步任务")
		return nil
	}
	// 确保函数退出时释放锁
	defer func() {
		if releaseErr := dao.App.KeywordsDb.ReleaseSyncLock(ctx, agentID); releaseErr != nil {
			global.Log.Errorf("释放Redis同步锁失败: %v", releaseErr)
		}
	}()

	global.Log.Debugln("获取同步锁成功，开始执行关键词重同步任务...")

	if global.ChatwootService == nil {
		return errors.New("Chatwoot客户端未初始化")
	}

	// 1. 获取上次同步的时间戳
	var lastSyncTime time.Time
	if global.RedisClient != nil {
		lastSyncTimeStr, _ := global.RedisClient.Get(ctx, redis.KeyLastSyncCannedResponses).Result()
		if parsedTime, err := time.Parse(time.RFC3339Nano, lastSyncTimeStr); err == nil {
			lastSyncTime = parsedTime
		}
	}

	// 增加“自冷却”机制，避免短时间内重复执行
	minSyncInterval := time.Duration(global.Config.Ai.KeywordSyncInterval) * time.Second
	if !lastSyncTime.IsZero() && time.Since(lastSyncTime) < minSyncInterval {
		global.Log.Infof("距离上次成功同步不足 %v，跳过本次任务", minSyncInterval)
		return nil
	}

	// 2. 从Chatwoot拉取全量数据
	allResponses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return fmt.Errorf("从Chatwoot获取预设回复失败: %w", err)
	}

	// 3. 分类，并找出需要增量处理的条目和所有ID
	newLatestSyncTime := lastSyncTime
	var responsesToProcess, exactMatchRules []chatwoot.CannedResponse
	var allSemanticIDs []string

	for _, resp := range allResponses {
		updatedAt, _ := time.Parse(time.RFC3339Nano, resp.UpdatedAt)
		if updatedAt.IsZero() {
			updatedAt = time.Now().In(global.Tz)
		}
		if updatedAt.After(newLatestSyncTime) {
			newLatestSyncTime = updatedAt
		}

		qType, qText := m.parseShortCode(resp.ShortCode)

		if qType == enum.KeywordTypeSemantic || qType == enum.KeywordTypeHybrid {
			allSemanticIDs = append(allSemanticIDs, fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, resp.Id))
			if updatedAt.After(lastSyncTime) {
				responsesToProcess = append(responsesToProcess, resp)
			}
		}
		if qType == enum.KeywordTypeExact || qType == enum.KeywordTypeHybrid {
			if qText != "" {
				rule := resp
				rule.ShortCode = strings.ToLower(qText)
				exactMatchRules = append(exactMatchRules, rule)
			}
		}
	}

	// 4. 对增量数据进行处理和缓存
	var processErr error
	if len(responsesToProcess) > 0 {
		global.Log.Infof("检测到 %d 条新增/修改的语义规则，开始处理...", len(responsesToProcess))
		if err := m.ProcessAndCacheResponses(ctx, responsesToProcess); err != nil {
			processErr = fmt.Errorf("处理增量规则失败: %w", err)
			global.Log.Error(processErr.Error())
		}
	}

	// 5. 对全量数据进行同步和清理（Redis、内存、ChromaDB清理）
	var syncErr error
	var wg errgroup.Group
	wg.Go(func() error {
		// 使用全量精确匹配规则重建Redis和内存缓存，确保数据完全一致
		return m.syncExactMatchCache(ctx, exactMatchRules)
	})
	wg.Go(func() error {
		// 清理向量数据库中已失效的条目
		return m.pruneVectorDb(ctx, allSemanticIDs)
	})
	if err := wg.Wait(); err != nil {
		syncErr = err
	}

	// 6. 仅在所有环节都成功时，才更新同步时间戳
	if processErr == nil && syncErr == nil {
		if global.RedisClient != nil && newLatestSyncTime.After(lastSyncTime) {
			global.RedisClient.Set(ctx, redis.KeyLastSyncCannedResponses, newLatestSyncTime.Format(time.RFC3339Nano), 0)
			global.Log.Debugln("同步时间戳已更新为: %s", newLatestSyncTime.Format(time.RFC3339Nano))
		}
	} else {
		global.Log.Warn("由于同步过程中发生错误，本次将不更新同步时间戳，以便下次重试")
	}

	global.Log.Info("关键词重同步任务完成")
	return syncErr
}

// ProcessAndCacheResponses 处理并缓存一个给定的 CannedResponse 列表。
// 这是一个可复用的核心方法，用于实时更新和批量同步。
func (m *Manager) ProcessAndCacheResponses(ctx context.Context, responses []chatwoot.CannedResponse) error {
	if len(responses) == 0 {
		return nil
	}

	var semanticRules, exactMatchRules []chatwoot.CannedResponse
	for _, resp := range responses {
		qType, qText := m.parseShortCode(resp.ShortCode)
		if qText == "" {
			continue
		}
		if qType == enum.KeywordTypeSemantic || qType == enum.KeywordTypeHybrid {
			semanticRules = append(semanticRules, resp)
		}
		if qType == enum.KeywordTypeExact || qType == enum.KeywordTypeHybrid {
			rule := resp
			rule.ShortCode = strings.ToLower(qText)
			exactMatchRules = append(exactMatchRules, rule)
		}
	}

	var documentsForVectorDB []vector.Document
	if len(semanticRules) > 0 {
		var err error
		documentsForVectorDB, err = m.generateVectorDocs(ctx, semanticRules)
		if err != nil {
			return err
		}
	}

	var wg errgroup.Group
	// 并发更新向量数据库
	if len(documentsForVectorDB) > 0 {
		wg.Go(func() error {
			if _, err := dao.App.VectorDb.BatchUpsert(ctx, documentsForVectorDB); err != nil {
				return fmt.Errorf("精准更新向量数据库失败: %w", err)
			}
			global.Log.Debugf("成功精准更新 %d 条规则到向量数据库", len(documentsForVectorDB))
			return nil
		})
	}
	// 并发更新精确匹配缓存
	if len(exactMatchRules) > 0 {
		wg.Go(func() error {
			// 在实时更新场景下，直接HSet即可，无需先删除。
			if global.RedisClient != nil {
				if _, err := dao.App.KeywordsDb.SaveKeywordsToRedis(ctx, exactMatchRules); err != nil {
					return fmt.Errorf("精准更新精确匹配规则到Redis失败: %w", err)
				}
			}
			m.updateInMemoryMap(exactMatchRules)
			global.Log.Debugf("成功精准更新 %d 条规则到精确匹配缓存", len(exactMatchRules))
			return nil
		})
	}

	return wg.Wait()
}

// generateVectorDocs 为语义规则生成向量文档。
func (m *Manager) generateVectorDocs(ctx context.Context, rules []chatwoot.CannedResponse) ([]vector.Document, error) {
	if global.LlmService == nil || global.EmbeddingService == nil {
		return nil, errors.New("LLM或Embedding服务未初始化")
	}

	type semanticJob struct {
		resp             chatwoot.CannedResponse
		standardQuestion string
	}
	var completedJobs []semanticJob
	var mu sync.Mutex
	var llmGroup errgroup.Group
	llmGroup.SetLimit(10)

	for _, resp := range rules {
		r := resp // 避免闭包陷阱
		llmGroup.Go(func() error {
			_, seedQuestion := m.parseShortCode(r.ShortCode)
			if seedQuestion == "" {
				return nil
			}

			standardQuestion, err := global.LlmService.GenerateStandardQuestion(ctx, enum.SystemPromptGenQuestionFromKeyword, seedQuestion)
			if err != nil {
				global.Log.Warnf("为ID %d 的内容生成标准问题失败: %v", r.Id, err)
				return nil
			}
			standardQuestion = strings.Trim(standardQuestion, `"'。， `)
			if standardQuestion == "" {
				return nil
			}

			mu.Lock()
			completedJobs = append(completedJobs, semanticJob{resp: r, standardQuestion: standardQuestion})
			mu.Unlock()
			return nil
		})
	}
	_ = llmGroup.Wait()

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
	embeddings, err := global.EmbeddingService.CreateEmbeddings(embedCtx, questionsToEmbed)
	if err != nil {
		return nil, fmt.Errorf("批量创建向量失败: %w", err)
	}

	var documents []vector.Document
	for i, job := range completedJobs {
		doc := vector.Document{
			ID:        fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, job.resp.Id),
			Metadata:  map[string]interface{}{
				dao.VectorMetadataKeyQuestion: job.standardQuestion,
				dao.VectorMetadataKeyAnswer:   job.resp.Content,
				dao.VectorMetadataKeySourceID: int64(job.resp.Id),
			},
			Embedding: embeddings[i],
		}
		documents = append(documents, doc)
	}
	return documents, nil
}

// syncExactMatchCache 使用给定的规则列表重建Redis和内存中的精确匹配缓存。
func (m *Manager) syncExactMatchCache(ctx context.Context, exactMatchRules []chatwoot.CannedResponse) error {
	// 1. 从精确匹配规则构建map
	tempMap := make(map[string]string, len(exactMatchRules))
	for _, rule := range exactMatchRules {
		// 假设传入的 rule.ShortCode 已经是处理过的
		tempMap[rule.ShortCode] = rule.Content
	}

	// 2. 重建Redis缓存
	if global.RedisClient != nil {
		// a. 删除旧的Hash
		if err := global.RedisClient.Del(ctx, redis.KeyCannedResponsesHash).Err(); err != nil {
			return fmt.Errorf("重建Redis缓存时删除旧Hash失败: %w", err)
		}
		// b. 写入新数据
		if len(exactMatchRules) > 0 {
			if _, err := dao.App.KeywordsDb.SaveKeywordsToRedis(ctx, exactMatchRules); err != nil {
				return fmt.Errorf("重建Redis缓存时写入新数据失败: %w", err)
			}
		}
	}

	// 3. 原子性地替换内存Map
	global.CannedResponses.Lock()
	global.CannedResponses.Data = tempMap
	global.CannedResponses.Unlock()

	global.Log.Infof("成功加载 %d 条精确匹配关键词到内存和Redis", len(tempMap))
	return nil
}


// pruneVectorDb 清理向量数据库中不再存在的条目。
func (m *Manager) pruneVectorDb(ctx context.Context, activeIDs []string) error {
	if global.VectorDb == nil {
		return nil // 如果没有向量数据库，则静默忽略
	}
	prunedCount, err := dao.App.VectorDb.PruneStale(ctx, activeIDs)
	if err != nil {
		global.Log.Warnf("清理向量数据库中过期条目失败: %v", err)
	}
	if prunedCount > 0 {
		global.Log.Printf("成功从向量数据库中清理 %d 条过期条目", prunedCount)
	}
	return nil
}

// updateInMemoryMap 原子性地更新或追加内存中的 CannedResponses Map
func (m *Manager) updateInMemoryMap(responses []chatwoot.CannedResponse) {
	global.CannedResponses.Lock()
	defer global.CannedResponses.Unlock()
	for _, resp := range responses {
		global.CannedResponses.Data[resp.ShortCode] = resp.Content
	}
}

// parseShortCode 是一个统一的 short_code 解析辅助函数
func (m *Manager) parseShortCode(shortCode string) (qType enum.KeywordType, qText string) {
	hybridPrefix := global.Config.Ai.HybridPrefix
	semanticPrefix := global.Config.Ai.SemanticPrefix

	if hybridPrefix != "" && strings.HasPrefix(shortCode, hybridPrefix) {
		return enum.KeywordTypeHybrid, strings.TrimPrefix(shortCode, hybridPrefix)
	}
	if strings.HasPrefix(shortCode, semanticPrefix) {
		return enum.KeywordTypeSemantic, strings.TrimPrefix(shortCode, semanticPrefix)
	}
	return enum.KeywordTypeExact, shortCode
}


// LoadKeywords 从Redis加载关键词到内存，并处理分布式锁
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
