package vector

import (
	"context"
	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

// Document 是一个通用的向量文档结构体，用于在应用内部传递数据
type Document struct {
	ID        string
	Metadata  map[string]interface{}
	Embedding []float32
}

// Service 定义了向量数据库服务的接口，封装了底层客户端
type Service interface {
	Heartbeat(ctx context.Context) error
	Close() error
	GetOrCreateCollection(ctx context.Context, name string) (chroma.Collection, error)
	// 批量插入或更新文档到指定的集合中
	Upsert(ctx context.Context, collectionName string, documents []Document) error
	// 根据ID批量删除文档
	DeleteByIDs(ctx context.Context, collectionName string, ids []string) (int, error)
}

type client struct {
	client chroma.Client
}

// NewClient 创建一个新的ChromaDB v2客户端实例
func NewClient(baseURL, authToken string) (Service, error) {
	clientOptions := []chroma.ClientOption{
		chroma.WithBaseURL(baseURL),
	}

	// 如果提供了authToken，则配置认证
	if authToken != "" {
		provider := chroma.NewTokenAuthCredentialsProvider(authToken, chroma.AuthorizationTokenHeader)
		clientOptions = append(clientOptions, chroma.WithAuth(provider))
	}

	cli, err := chroma.NewHTTPClient(clientOptions...)
	if err != nil {
		return nil, err
	}

	return &client{
		client: cli,
	}, nil
}

func (c *client) Heartbeat(ctx context.Context) error {
	return c.client.Heartbeat(ctx)
}

func (c *client) Close() error {
	return c.client.Close()
}

type NoOpEmbeddingFunction struct{}

func (f *NoOpEmbeddingFunction) EmbedDocuments(ctx context.Context, texts []string) ([]embeddings.Embedding, error) {
	return make([]embeddings.Embedding, len(texts)), nil
}

func (f *NoOpEmbeddingFunction) EmbedQuery(ctx context.Context, text string) (embeddings.Embedding, error) {
	return nil, nil
}

func (c *client) GetOrCreateCollection(ctx context.Context, name string) (chroma.Collection, error) {
	// 使用自定义的 NoOpEmbeddingFunction 来覆盖默认的嵌入函数，防止在静态编译环境下因加载 onnxruntime 而导致 cgo 相关的 SIGSEGV 错误。
	col, err := c.client.GetOrCreateCollection(ctx, name, chroma.WithEmbeddingFunctionCreate(&NoOpEmbeddingFunction{}))
	if err != nil {
		return nil, err
	}
	return col, nil
}

func (c *client) Upsert(ctx context.Context, collectionName string, documents []Document) error {
	if len(documents) == 0 {
		return nil
	}

	col, err := c.GetOrCreateCollection(ctx, collectionName)
	if err != nil {
		return err
	}

	// 将内部的Document结构转换为ChromaDB v2 API所需的类型
	documentIDs := make([]chroma.DocumentID, len(documents))
	var chromaMetadatas []chroma.DocumentMetadata
	var chromaEmbeddings []embeddings.Embedding

	for i, doc := range documents {
		documentIDs[i] = chroma.DocumentID(doc.ID)
		chromaMetadatas = append(chromaMetadatas, chroma.NewMetadataFromMap(doc.Metadata))
		chromaEmbeddings = append(chromaEmbeddings, embeddings.NewEmbeddingFromFloat32(doc.Embedding))
	}

	return col.Upsert(
		ctx,
		chroma.WithIDs(documentIDs...),
		chroma.WithMetadatas(chromaMetadatas...),
		chroma.WithEmbeddings(chromaEmbeddings...),
	)
}

func (c *client) DeleteByIDs(ctx context.Context, collectionName string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	col, err := c.GetOrCreateCollection(ctx, collectionName)
	if err != nil {
		return 0, err
	}

	docIDs := make([]chroma.DocumentID, len(ids))
	for i, id := range ids {
		docIDs[i] = chroma.DocumentID(id)
	}

	err = col.Delete(ctx, chroma.WithIDsDelete(docIDs...))
	if err != nil {
		return 0, err
	}

	return len(ids), nil
}
