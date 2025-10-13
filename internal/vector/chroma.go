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
	Upsert(ctx context.Context, collectionName string, documents []Document) error
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

func (c *client) GetOrCreateCollection(ctx context.Context, name string) (chroma.Collection, error) {
	// TODO: 未来可以根据需要进行参数化配置
	col, err := c.client.GetOrCreateCollection(ctx, name)
	if err != nil {
		return nil, err
	}
	return col, nil
}

// Upsert 批量插入或更新文档到指定的集合中
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
