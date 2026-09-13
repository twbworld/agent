# Chatwoot AI Agent

[![CI](https://github.com/twbworld/agent/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/twbworld/agent/actions/workflows/ci.yaml)
[![](https://img.shields.io/github/tag/twbworld/agent?logo=github)](https://github.com/twbworld/agent)
![](https://img.shields.io/badge/language-golang-cyan)
[![](https://img.shields.io/github/license/twbworld/agent)](https://github.com/twbworld/agent/blob/main/LICENSE)

**Chatwoot AI Agent** 是一款基于 Golang 开发的无状态的高性能智能客服中间件。

作为 Chatwoot 的“智能大脑”，本项目致力于实现“AI 优先，人机协同”的现代化客服工作流。深度集成了 **MCP (Model Context Protocol)** 与大型语言模型，实现了从精准回复、知识库检索 (RAG) 到多轮复杂推理 (ReAct) 的智能客服全生命周期自动化。

---

## ✨ 核心特性

- **🧠 多级智能路由**：构建了 `精确匹配 (Redis)` -> `语义检索 (向量库)` -> `小模型分诊台 (Triage)` -> `大模型推理 (ReAct)` 的漏斗型路由机制，在保证回答质量的同时，大幅降低了大模型 Token 成本并提升了响应速度。
- **🔌 原生 MCP 工具集成**：支持 Model Context Protocol 协议，大模型可无缝并动态地调用外部业务系统（如查询电商订单、商品库、用户信息等）。
- **🤝 无缝人机协作 (Human-in-the-loop)**：内置情绪识别与高危意图监控。首创 **人工宽限期 (Grace Period)** 机制，当人工客服介入时 AI 自动“静默”，人工离线后 AI 自动重新接管。
- **⚡ 企业级高可用设计**：全盘采用 Golang 编写，利用 Redis 分布式锁、上下文超时控制 (Context) 与 Webhook 幂等校验，彻底杜绝高并发场景下的消息雪崩与重复回复。

## 🗺️ 发展规划 (Roadmap)

为了打造更加稳定、可扩展的 AI 架构，项目未来的核心演进计划如下：

- [ ] **控制流重构为状态机**：将 `controller/user/chat.go` 中复杂的面条式控制流重构为严格的**状态机（State Machine / 状态图）**，使多轮会话和状态流转（等待输入、工具执行中、转人工等）具高度可预测性。
- [ ] **接入 `cloudwego/eino`**：全面嵌入字节跳动开源的 Golang AI 应用框架 [`cloudwego/eino`](https://github.com/cloudwego/eino)。利用 Eino 的 Graph 编排能力取代手写的 ReAct 循环，实现更规范的 Agent 协同与可观测性。
- [ ] **架构解耦**：重构 Controller 层，剥离过度集中的业务逻辑，进一步解耦路由分发与底层服务。
- [ ] **适配 MCP 2.0**：紧跟开源生态标准，升级并适配 MCP 2.0 协议特性。
- [ ] **工业级向量数据库升级**：逐步使用 **Qdrant** 或 **Milvus** 替换当前的 ChromaDB 方案，应对海量企业知识库的检索需求。

## 📂 项目结构

```text
./
├── config.yaml          # 项目核心配置文件
├── controller/          # 路由控制层 (涵盖 Chatwoot Webhook 入口)
│   ├── admin/           # 后台管理 API
│   └── user/            # 用户端 Webhook 核心逻辑 (chat.go)
├── dao/                 # 数据访问层 (Redis/向量数据库等抽象)
├── internal/            # 核心底层组件 (MCP Client, OpenAI LLM, Chatwoot API等)
├── model/               # 数据模型、通用结构体与常量枚举
├── service/             # 业务逻辑服务层 (行为控制 Action, 会话历史 History, 大模型交互等)
├── static/              # Dashboard 等前端静态资源
└── task/                # 后台异步任务 (知识库后台重载、MCP 能力同步等)
```

## 🚀 快速开始

### 1. 环境准备
- Go 1.27+
- Redis (用于分布式锁、历史记忆、宽限期控制等)
- ChromaDB (向量检索)
- Chatwoot 平台实例

### 2. 配置文件
复制并修改配置模板：
```bash
cp config.example.yaml config.yaml
```
根据您的环境填写 `config.yaml` 中的 Chatwoot API Token、大模型密钥、MCP 服务地址以及 Redis/数据库连接信息。

### 3. 启动服务
```bash
# 源码启动
go run main.go

# 或通过 Docker 容器化运行
docker build -t chatwoot-ai-agent .
docker run -d -p 8080:8080 chatwoot-ai-agent

# 或通过 Docker 镜像启动
docker run -d \
  --name chatwoot-ai-agent \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  ghcr.io/twbworld/agent:latest
```

### 4. Chatwoot Webhook 配置
在 Chatwoot 设置中新增 Webhook，将事件回调地址指向本服务的对外接口（例如：`http://您的域名/api/user/chat`），勾选所需的事件订阅（如 `message_created`, `webwidget_triggered`）。
