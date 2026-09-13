## 一、Agent 全局行为准则

### 1.1 角色身份
- 资深 Go 语言后端架构师与AI 应用开发专家，协助开发**商城智能 AI 客服系统（核心调度/AI编排服务）**。

### 1.2 行为准则
- 全程使用**简体中文**交互
- 严格遵循项目架构、技术栈、业务规范与编码标准，输出严谨专业的代码与方案。
- 开始任何开发任务前，优先查阅 `README.md`。
- 注释统一使用中文，力求极简通俗，仅在复杂业务及关键处补充；禁止无关注释，非必要不增改现有注释。
- 涉及未知依赖、外部接口或存在时效性疑问时，必要时主动联网检索以获取最新、准确的信息。

---

## 二、Go 编码规范

### 2.1 语法与性能
- **新特性使用**：结合 `go.mod` 尽量采用 Go(>1.27) 语法, 如:
    - 泛型
    - new(expr)
    - 协程可直接闭包引用循环变量
    - 结构体字面量内嵌字段平铺赋值
    - 善用 `json:"-"`、`omitempty` 、 `omitzero`
    - encoding/json/v2
    - cmp包, 如`cmp.Or()`
- **内存与切片优化**：
  - 字符串少量拼接用 `+`，循环或大批量拼接用 `strings.Builder`（配合 Grow 预分配）。
  - `make` slice/map 尽量预设容量，避免扩容开销。
  - 从大切片截取小片段时，优先使用 `slices.Clone` 避免底层大数组无法被 GC 回收。
- **并发与事务**：充分利用协程提高吞吐，权衡评估 `chan` 的必要性；事务内严禁执行非必要查询与外部网络 I/O。
- **命名规范**：遵循 Go 官方命名哲学，局部短作用域推荐地道短命名；中长作用域与包级命名要简明规范。

### 2.2 并发安全
- **协程控制与恢复**：推荐使用 `errgroup` 统一管控协程；裸 `go` 协程内部必须配备 `defer/recover` 防御 panic 导致主进程崩溃。
- **sync使用**：善用 `sync` 原语，读多写少优先 `RWMutex` 并最小化临界区；单例与延迟初始化优先使用 `sync.OnceValue` / `sync.OnceValues`；高频临时对象使用 `sync.Pool` 结合 `clear()` 降 GC。

### 2.3 架构、测试与设计模式
- **可测性与依赖注入 (DI)**：架构设计必须面向接口与单元测试友好，依赖项（如 DB）优先显式注入，杜绝直接读取全局状态。
- **单测同步更新**：新增或修改功能务必同步更新单元测试。
- **上下文传递**：所有阻塞操作与 I/O 调用的首个参数必须为 `context.Context`，用于链路控制与超时取消。
- **防御性编程**：
  - 严格检查所有 `error`，数据库查询必须显式处理 `sql.ErrNoRows`。
  - 当 Map 的零值具有歧义时，必须使用 `val, ok := m[k]` 区分键是否存在；外部输入与配置对象必做 `nil` 空指针防御。
- **全局设计原则**：保持高内聚、低耦合、规范，杜绝过度设计，追求 Idiomatic Go 的清晰与极简。

### 2.4 禁止事项
- **严禁使用 `fmt` 打印业务日志**：必须统一使用项目配置的结构化日志库。
- **禁止滥用 `init`**：初始化逻辑收敛至构造函数或显式启动链路。
- **严禁定义运行时可变全局变量**。
- **禁止依赖 Map 遍历顺序**：永远将 `for range` 处理的 map 视为无序。

---

## 三、项目级规范

### 3.1 缓存规范
- 严格规避 Redis **缓存穿透、缓存击穿、缓存雪崩**；涉及分布式同步任务时必须使用分布式锁。

### 3.2 配置同步规范
- 变更配置文件时，必须同步更新以下关联文件：
  - 示例配置：`config.example.yaml`
  - 模型定义：`model/config/default.go` 及 `model/config/child.go`
  - 默认值处理：`initialize/config.go` 下的 `handleConfig()`
  - 热重载处理：`initialize/config.go` 下的 `HandleConfigChange()`

### 3.3 代码复用
- 优先复用 `utils/tool.go` 内的通用工具函数，避免重复造轮子。

### 3.4 LLM 工程与提示词规范
- **提示词结构**: 涉及 LLM Prompt 相关编码时，**务必优化提示词结构顺序**：Context(静态属性) -> Docs(RAG动态) -> Time(时刻) -> Question(极高频变化)，最大化提升 LLM 平台的 **KV Cache 命中率**以降低延迟。
- **交互与能力调用**：涉及 LLM 交互业务时，优先考虑使用结构化输出 (Structured Outputs) 与 Function Calling 机制，确保输出格式确定性与执行可控性。

---
## 四、系统架构与边界

本项目定位为 **AI 调度与编排服务 (Go + Gin)**，负责会话决策中枢，严禁跨层编写底层业务逻辑。

1. **边界划分**：
   - **交互层 (Chatwoot)**：负责消息收发、坐席工作台；通过 Webhook 推送至本项目，通过 REST API 接收回复与转人工指令。
   - **调度层 (本项目)**：负责消息解析、意图分诊、RAG 检索、MCP 调用编排与转人工裁决。
   - **执行层 (MCP 服务)**：**独立外部微服务**（非本项目代码），本项目仅通过 Streamable HTTP 协议调用其工具接口。
2. **底层基础设施**：
   - **LLM**：本地 Qwen 系列（轻量模型用于分诊/意图识别，主力模型用于 RAG 综合与工具生成）。
   - **向量库**：`ChromaDB`（知识库语义检索）。
   - **多级缓存**：`Redis`（快捷回复源、分布式锁）+ 内存 `sync.Map`（一级热点读缓存）。

---

## 五、核心流转机制与协议规范

### 5.1 知识库同步机制
- **全量审计 (`KeywordReloader`)**：定时全量校准 Chatwoot 数据至 Redis、Chroma 及内存，确保最终一致性。
- **实时更新 API (`keyword.html`)**：
  - **删除**：精准清理内存、Redis、Chroma 对应条目。
  - **新增/修改**：实时调用轻量 LLM 泛化标准问题并写入各级缓存；附加防抖延迟审计任务（防抖时长由配置项指定，禁止硬编码）。

### 5.2 快捷回复 ShortCode 协议
路由匹配严格遵循 ShortCode 前缀规范，禁止混淆存储介质：
- **精确匹配 (`keyword`)**：无 `ai@` 前缀。仅写入内存/Redis，跳过向量化与 LLM 处理。
- **语义匹配 (`ai@<seed>` 或 `ai@`)**：经轻量 LLM 提炼为自然语言标准问后写入 Chroma，禁止写入精确匹配缓存。
- **混合匹配 (`ai+@<keyword>`)**：同时执行精确匹配入库与 LLM 泛化向量化。

### 5.3 对话调度与转人工机制
消息按流水线处理，优先短路返回：
1. **快速路径**：内存精确匹配命中或向量库高相似度命中（阈值走配置）直接返回，终止流程。
2. **智能分诊**：轻量 LLM 评估意图、情绪与风险；闲聊直接礼貌拒绝；**情绪激动、明确转人工、命中高危规则（如大额资损/严重投诉，阈值走配置）立即转人工**。
3. **复杂编排 (RAG + LLM + Tool)**：组装对话历史与向量检索片段；命中 `<tool_code>` 时调用外部 MCP 服务，结果回填后闭环生成最终回复。
4. **兜底转人工**：LLM 返回 `I_AM_UNSURE...`、单问题重试超限（阈值走配置）、富媒体消息（图片/语音）时，一律平滑流转人工。



# Chatwoot-webhook数据结构

```json
{
    "account": {
        "id": 1,
        "name": "tj"
    },
    "additional_attributes": {},
    "avatar": "",
    "custom_attributes": {
        "goods_id": "prod_12345",
        "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
        "goods_title": "Qwen3-72B 限量版纪念T恤"
    },
    "email": null,
    "id": 163,
    "identifier": null,
    "name": "billowing-night-17",
    "phone_number": null,
    "thumbnail": "",
    "blocked": false,
    "event": "contact_updated",
    "changed_attributes": [
        {
            "updated_at": {
                "previous_value": "2025-11-15T03:32:25.810Z",
                "current_value": "2025-11-15T03:32:26.292Z"
            }
        },
        {
            "custom_attributes": {
                "previous_value": {},
                "current_value": {
                    "goods_id": "prod_12345",
                    "goods_title": "Qwen3-72B 限量版纪念T恤",
                    "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png"
                }
            }
        }
    ]
}
```

点击浮窗:
```json
{
    "id": 163,
    "contact": {
        "account": {
            "id": 1,
            "name": "tj"
        },
        "additional_attributes": {},
        "avatar": "",
        "custom_attributes": {
            "goods_id": "prod_12345",
            "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
            "goods_title": "Qwen3-72B 限量版纪念T恤"
        },
        "email": null,
        "id": 163,
        "identifier": null,
        "name": "billowing-night-17",
        "phone_number": null,
        "thumbnail": "",
        "blocked": false
    },
    "inbox": {
        "id": 1,
        "name": "淘街"
    },
    "account": {
        "id": 1,
        "name": "tj"
    },
    "current_conversation": null,
    "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3",
    "event": "webwidget_triggered",
    "event_info": {
        "initiated_at": {
            "timestamp": "Sat Nov 15 2025 11:33:32 GMT+0800 (中国标准时间)"
        },
        "referer": "http://agent.cc.cc/",
        "widget_language": "zh_CN",
        "browser_language": "zh",
        "browser": {
            "browser_name": "Microsoft Edge",
            "browser_version": "142.0.0.0",
            "device_name": "Unknown",
            "platform_name": "Windows",
            "platform_version": "10.0"
        }
    }
}
```

```json
{
    "additional_attributes": {
        "browser": {
            "device_name": "Unknown",
            "browser_name": "Microsoft Edge",
            "platform_name": "Windows",
            "browser_version": "142.0.0.0",
            "platform_version": "10.0"
        },
        "referer": "http://agent.cc.cc/",
        "initiated_at": {
            "timestamp": "Sat Nov 15 2025 11:35:11 GMT+0800 (中国标准时间)"
        },
        "browser_language": "zh"
    },
    "can_reply": true,
    "channel": "Channel::WebWidget",
    "contact_inbox": {
        "id": 163,
        "contact_id": 163,
        "inbox_id": 1,
        "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3",
        "created_at": "2025-11-15T03:32:25.816Z",
        "updated_at": "2025-11-15T03:32:25.816Z",
        "hmac_verified": false,
        "pubsub_token": "zJJMyipUDmJCzHAreZD1XyeH"
    },
    "id": 150,
    "inbox_id": 1,
    "messages": [
        {
            "id": 810,
            "content": "你好",
            "account_id": 1,
            "inbox_id": 1,
            "conversation_id": 150,
            "message_type": 0,
            "created_at": 1763177714,
            "updated_at": "2025-11-15T03:35:14.179Z",
            "private": false,
            "status": "sent",
            "source_id": null,
            "content_type": "text",
            "content_attributes": {
                "in_reply_to": null
            },
            "sender_type": "Contact",
            "sender_id": 163,
            "external_source_ids": {},
            "additional_attributes": {},
            "processed_message_content": "你好",
            "sentiment": {},
            "conversation": {
                "assignee_id": null,
                "unread_count": 1,
                "last_activity_at": 1763177714,
                "contact_inbox": {
                    "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3"
                }
            },
            "sender": {
                "additional_attributes": {},
                "custom_attributes": {
                    "goods_id": "prod_12345",
                    "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                    "goods_title": "Qwen3-72B 限量版纪念T恤"
                },
                "email": null,
                "id": 163,
                "identifier": null,
                "name": "billowing-night-17",
                "phone_number": null,
                "thumbnail": "",
                "blocked": false,
                "type": "contact"
            }
        }
    ],
    "labels": [],
    "meta": {
        "sender": {
            "additional_attributes": {},
            "custom_attributes": {
                "goods_id": "prod_12345",
                "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                "goods_title": "Qwen3-72B 限量版纪念T恤"
            },
            "email": null,
            "id": 163,
            "identifier": null,
            "name": "billowing-night-17",
            "phone_number": null,
            "thumbnail": "",
            "blocked": false,
            "type": "contact"
        },
        "assignee": null,
        "team": null,
        "hmac_verified": false
    },
    "status": "pending",
    "custom_attributes": {},
    "snoozed_until": null,
    "unread_count": 1,
    "first_reply_created_at": null,
    "priority": null,
    "waiting_since": 1763177714,
    "agent_last_seen_at": 0,
    "contact_last_seen_at": 0,
    "last_activity_at": 1763177714,
    "timestamp": 1763177714,
    "created_at": 1763177714,
    "updated_at": 1763177714.155967,
    "event": "conversation_created"
}
```

```json
{
    "account": {
        "id": 1,
        "name": "tj"
    },
    "additional_attributes": {},
    "content_attributes": {
        "in_reply_to": null
    },
    "content_type": "text",
    "content": "你好",
    "conversation": {
        "additional_attributes": {
            "browser_language": "zh",
            "browser": {
                "browser_name": "Microsoft Edge",
                "browser_version": "142.0.0.0",
                "device_name": "Unknown",
                "platform_name": "Windows",
                "platform_version": "10.0"
            },
            "initiated_at": {
                "timestamp": "Sat Nov 15 2025 11:35:11 GMT+0800 (中国标准时间)"
            },
            "referer": "http://agent.cc.cc/"
        },
        "can_reply": true,
        "channel": "Channel::WebWidget",
        "contact_inbox": {
            "id": 163,
            "contact_id": 163,
            "inbox_id": 1,
            "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3",
            "created_at": "2025-11-15T03:32:25.816Z",
            "updated_at": "2025-11-15T03:32:25.816Z",
            "hmac_verified": false,
            "pubsub_token": "zJJMyipUDmJCzHAreZD1XyeH"
        },
        "id": 150,
        "inbox_id": 1,
        "messages": [
            {
                "id": 810,
                "content": "你好",
                "account_id": 1,
                "inbox_id": 1,
                "conversation_id": 150,
                "message_type": 0,
                "created_at": 1763177714,
                "updated_at": "2025-11-15T03:35:14.179Z",
                "private": false,
                "status": "sent",
                "source_id": null,
                "content_type": "text",
                "content_attributes": {
                    "in_reply_to": null
                },
                "sender_type": "Contact",
                "sender_id": 163,
                "external_source_ids": {},
                "additional_attributes": {},
                "processed_message_content": "你好",
                "sentiment": {},
                "conversation": {
                    "assignee_id": null,
                    "unread_count": 1,
                    "last_activity_at": 1763177714,
                    "contact_inbox": {
                        "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3"
                    }
                },
                "sender": {
                    "additional_attributes": {},
                    "custom_attributes": {
                        "goods_id": "prod_12345",
                        "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                        "goods_title": "Qwen3-72B 限量版纪念T恤"
                    },
                    "email": null,
                    "id": 163,
                    "identifier": null,
                    "name": "billowing-night-17",
                    "phone_number": null,
                    "thumbnail": "",
                    "blocked": false,
                    "type": "contact"
                }
            }
        ],
        "labels": [],
        "meta": {
            "sender": {
                "additional_attributes": {},
                "custom_attributes": {
                    "goods_id": "prod_12345",
                    "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                    "goods_title": "Qwen3-72B 限量版纪念T恤"
                },
                "email": null,
                "id": 163,
                "identifier": null,
                "name": "billowing-night-17",
                "phone_number": null,
                "thumbnail": "",
                "blocked": false,
                "type": "contact"
            },
            "assignee": null,
            "team": null,
            "hmac_verified": false
        },
        "status": "pending",
        "custom_attributes": {},
        "snoozed_until": null,
        "unread_count": 1,
        "first_reply_created_at": null,
        "priority": null,
        "waiting_since": 1763177714,
        "agent_last_seen_at": 0,
        "contact_last_seen_at": 0,
        "last_activity_at": 1763177714,
        "timestamp": 1763177714,
        "created_at": 1763177714,
        "updated_at": 1763177714.183048
    },
    "created_at": "2025-11-15T03:35:14.179Z",
    "id": 810,
    "inbox": {
        "id": 1,
        "name": "淘街"
    },
    "message_type": "incoming",
    "private": false,
    "sender": {
        "account": {
            "id": 1,
            "name": "tj"
        },
        "additional_attributes": {},
        "avatar": "",
        "custom_attributes": {
            "goods_id": "prod_12345",
            "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
            "goods_title": "Qwen3-72B 限量版纪念T恤"
        },
        "email": null,
        "id": 163,
        "identifier": null,
        "name": "billowing-night-17",
        "phone_number": null,
        "thumbnail": "",
        "blocked": false
    },
    "source_id": null,
    "event": "message_created"
}
```

```json
{
    "account": {
        "id": 1,
        "name": "tj"
    },
    "additional_attributes": {},
    "avatar": "",
    "custom_attributes": {
        "goods_id": "prod_12345",
        "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
        "goods_title": "Qwen3-72B 限量版纪念T恤"
    },
    "email": null,
    "id": 163,
    "identifier": null,
    "name": "billowing-night-17",
    "phone_number": null,
    "thumbnail": "",
    "blocked": false,
    "event": "contact_updated",
    "changed_attributes": [
        {
            "updated_at": {
                "previous_value": "2025-11-15T03:32:26.292Z",
                "current_value": "2025-11-15T03:35:14.232Z"
            }
        },
        {
            "last_activity_at": {
                "previous_value": null,
                "current_value": "2025-11-15T03:35:14.231Z"
            }
        }
    ]
}
```

```json
{
    "additional_attributes": {
        "browser": {
            "device_name": "Unknown",
            "browser_name": "Microsoft Edge",
            "platform_name": "Windows",
            "browser_version": "142.0.0.0",
            "platform_version": "10.0"
        },
        "referer": "http://agent.cc.cc/",
        "initiated_at": {
            "timestamp": "Sat Nov 15 2025 11:35:11 GMT+0800 (中国标准时间)"
        },
        "browser_language": "zh"
    },
    "can_reply": true,
    "channel": "Channel::WebWidget",
    "contact_inbox": {
        "id": 163,
        "contact_id": 163,
        "inbox_id": 1,
        "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3",
        "created_at": "2025-11-15T03:32:25.816Z",
        "updated_at": "2025-11-15T03:32:25.816Z",
        "hmac_verified": false,
        "pubsub_token": "zJJMyipUDmJCzHAreZD1XyeH"
    },
    "id": 150,
    "inbox_id": 1,
    "messages": [
        {
            "id": 810,
            "content": "你好",
            "account_id": 1,
            "inbox_id": 1,
            "conversation_id": 150,
            "message_type": 0,
            "created_at": 1763177714,
            "updated_at": "2025-11-15T03:35:14.179Z",
            "private": false,
            "status": "sent",
            "source_id": null,
            "content_type": "text",
            "content_attributes": {
                "in_reply_to": null
            },
            "sender_type": "Contact",
            "sender_id": 163,
            "external_source_ids": {},
            "additional_attributes": {},
            "processed_message_content": "你好",
            "sentiment": {},
            "conversation": {
                "assignee_id": null,
                "unread_count": 1,
                "last_activity_at": 1763177714,
                "contact_inbox": {
                    "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3"
                }
            },
            "sender": {
                "additional_attributes": {},
                "custom_attributes": {
                    "goods_id": "prod_12345",
                    "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                    "goods_title": "Qwen3-72B 限量版纪念T恤"
                },
                "email": null,
                "id": 163,
                "identifier": null,
                "name": "billowing-night-17",
                "phone_number": null,
                "thumbnail": "",
                "blocked": false,
                "type": "contact"
            }
        }
    ],
    "labels": [],
    "meta": {
        "sender": {
            "additional_attributes": {},
            "custom_attributes": {
                "goods_id": "prod_12345",
                "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                "goods_title": "Qwen3-72B 限量版纪念T恤"
            },
            "email": null,
            "id": 163,
            "identifier": null,
            "name": "billowing-night-17",
            "phone_number": null,
            "thumbnail": "",
            "blocked": false,
            "type": "contact"
        },
        "assignee": null,
        "team": null,
        "hmac_verified": false
    },
    "status": "pending",
    "custom_attributes": {},
    "snoozed_until": null,
    "unread_count": 1,
    "first_reply_created_at": null,
    "priority": null,
    "waiting_since": 1763177714,
    "agent_last_seen_at": 0,
    "contact_last_seen_at": 1763177714,
    "last_activity_at": 1763177714,
    "timestamp": 1763177714,
    "created_at": 1763177714,
    "updated_at": 1763177714.230474,
    "event": "conversation_status_changed",
    "changed_attributes": null
}
```

```json
{
    "additional_attributes": {
        "browser": {
            "device_name": "Unknown",
            "browser_name": "Microsoft Edge",
            "platform_name": "Windows",
            "browser_version": "142.0.0.0",
            "platform_version": "10.0"
        },
        "referer": "http://agent.cc.cc/",
        "initiated_at": {
            "timestamp": "Sat Nov 15 2025 11:35:11 GMT+0800 (中国标准时间)"
        },
        "browser_language": "zh"
    },
    "can_reply": true,
    "channel": "Channel::WebWidget",
    "contact_inbox": {
        "id": 163,
        "contact_id": 163,
        "inbox_id": 1,
        "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3",
        "created_at": "2025-11-15T03:32:25.816Z",
        "updated_at": "2025-11-15T03:32:25.816Z",
        "hmac_verified": false,
        "pubsub_token": "zJJMyipUDmJCzHAreZD1XyeH"
    },
    "id": 150,
    "inbox_id": 1,
    "messages": [
        {
            "id": 810,
            "content": "你好",
            "account_id": 1,
            "inbox_id": 1,
            "conversation_id": 150,
            "message_type": 0,
            "created_at": 1763177714,
            "updated_at": "2025-11-15T03:35:14.179Z",
            "private": false,
            "status": "sent",
            "source_id": null,
            "content_type": "text",
            "content_attributes": {
                "in_reply_to": null
            },
            "sender_type": "Contact",
            "sender_id": 163,
            "external_source_ids": {},
            "additional_attributes": {},
            "processed_message_content": "你好",
            "sentiment": {},
            "conversation": {
                "assignee_id": null,
                "unread_count": 1,
                "last_activity_at": 1763177714,
                "contact_inbox": {
                    "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3"
                }
            },
            "sender": {
                "additional_attributes": {},
                "custom_attributes": {
                    "goods_id": "prod_12345",
                    "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                    "goods_title": "Qwen3-72B 限量版纪念T恤"
                },
                "email": null,
                "id": 163,
                "identifier": null,
                "name": "billowing-night-17",
                "phone_number": null,
                "thumbnail": "",
                "blocked": false,
                "type": "contact"
            }
        }
    ],
    "labels": [],
    "meta": {
        "sender": {
            "additional_attributes": {},
            "custom_attributes": {
                "goods_id": "prod_12345",
                "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                "goods_title": "Qwen3-72B 限量版纪念T恤"
            },
            "email": null,
            "id": 163,
            "identifier": null,
            "name": "billowing-night-17",
            "phone_number": null,
            "thumbnail": "",
            "blocked": false,
            "type": "contact"
        },
        "assignee": null,
        "team": null,
        "hmac_verified": false
    },
    "status": "pending",
    "custom_attributes": {},
    "snoozed_until": null,
    "unread_count": 1,
    "first_reply_created_at": null,
    "priority": null,
    "waiting_since": 1763177714,
    "agent_last_seen_at": 0,
    "contact_last_seen_at": 1763177714,
    "last_activity_at": 1763177714,
    "timestamp": 1763177714,
    "created_at": 1763177714,
    "updated_at": 1763177714.230474,
    "event": "conversation_updated",
    "changed_attributes": [
        {
            "id": {
                "previous_value": null,
                "current_value": 150
            }
        },
        {
            "account_id": {
                "previous_value": null,
                "current_value": 1
            }
        },
        {
            "inbox_id": {
                "previous_value": null,
                "current_value": 1
            }
        },
        {
            "status": {
                "previous_value": "open",
                "current_value": "pending"
            }
        },
        {
            "created_at": {
                "previous_value": null,
                "current_value": "2025-11-15T03:35:14.155Z"
            }
        },
        {
            "updated_at": {
                "previous_value": null,
                "current_value": "2025-11-15T03:35:14.155Z"
            }
        },
        {
            "contact_id": {
                "previous_value": null,
                "current_value": 163
            }
        },
        {
            "additional_attributes": {
                "previous_value": {},
                "current_value": {
                    "browser_language": "zh",
                    "browser": {
                        "browser_name": "Microsoft Edge",
                        "browser_version": "142.0.0.0",
                        "device_name": "Unknown",
                        "platform_name": "Windows",
                        "platform_version": "10.0"
                    },
                    "initiated_at": {
                        "timestamp": "Sat Nov 15 2025 11:35:11 GMT+0800 (中国标准时间)"
                    },
                    "referer": "http://agent.cc.cc/"
                }
            }
        },
        {
            "contact_inbox_id": {
                "previous_value": null,
                "current_value": 163
            }
        },
        {
            "uuid": {
                "previous_value": null,
                "current_value": "1e6f3370-43ab-47fa-b3a1-41d4c617b87b"
            }
        },
        {
            "last_activity_at": {
                "previous_value": null,
                "current_value": "2025-11-15T03:35:14.151Z"
            }
        },
        {
            "waiting_since": {
                "previous_value": null,
                "current_value": "2025-11-15T03:35:14.155Z"
            }
        }
    ]
}
```

```json
{
    "event": "conversation_typing_off",
    "user": {
        "account": {
            "id": 1,
            "name": "tj"
        },
        "additional_attributes": {},
        "avatar": "",
        "custom_attributes": {
            "goods_id": "prod_12345",
            "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
            "goods_title": "Qwen3-72B 限量版纪念T恤"
        },
        "email": null,
        "id": 163,
        "identifier": null,
        "name": "billowing-night-17",
        "phone_number": null,
        "thumbnail": "",
        "blocked": false
    },
    "conversation": {
        "additional_attributes": {
            "browser": {
                "device_name": "Unknown",
                "browser_name": "Microsoft Edge",
                "platform_name": "Windows",
                "browser_version": "142.0.0.0",
                "platform_version": "10.0"
            },
            "referer": "http://agent.cc.cc/",
            "initiated_at": {
                "timestamp": "Sat Nov 15 2025 11:35:11 GMT+0800 (中国标准时间)"
            },
            "browser_language": "zh"
        },
        "can_reply": true,
        "channel": "Channel::WebWidget",
        "contact_inbox": {
            "id": 163,
            "contact_id": 163,
            "inbox_id": 1,
            "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3",
            "created_at": "2025-11-15T03:32:25.816Z",
            "updated_at": "2025-11-15T03:32:25.816Z",
            "hmac_verified": false,
            "pubsub_token": "zJJMyipUDmJCzHAreZD1XyeH"
        },
        "id": 150,
        "inbox_id": 1,
        "messages": [
            {
                "id": 810,
                "content": "你好",
                "account_id": 1,
                "inbox_id": 1,
                "conversation_id": 150,
                "message_type": 0,
                "created_at": 1763177714,
                "updated_at": "2025-11-15T03:35:14.179Z",
                "private": false,
                "status": "sent",
                "source_id": null,
                "content_type": "text",
                "content_attributes": {
                    "in_reply_to": null
                },
                "sender_type": "Contact",
                "sender_id": 163,
                "external_source_ids": {},
                "additional_attributes": {},
                "processed_message_content": "你好",
                "sentiment": {},
                "conversation": {
                    "assignee_id": null,
                    "unread_count": 1,
                    "last_activity_at": 1763177714,
                    "contact_inbox": {
                        "source_id": "f8cf369d-2d88-40b8-9253-887b675a33d3"
                    }
                },
                "sender": {
                    "additional_attributes": {},
                    "custom_attributes": {
                        "goods_id": "prod_12345",
                        "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                        "goods_title": "Qwen3-72B 限量版纪念T恤"
                    },
                    "email": null,
                    "id": 163,
                    "identifier": null,
                    "name": "billowing-night-17",
                    "phone_number": null,
                    "thumbnail": "",
                    "blocked": false,
                    "type": "contact"
                }
            }
        ],
        "labels": [],
        "meta": {
            "sender": {
                "additional_attributes": {},
                "custom_attributes": {
                    "goods_id": "prod_12345",
                    "goods_image": "https://wjyqtj.oss-cn-guangzhou.aliyuncs.com/temp/f966babe-031c-4ef7-b29f-ff4d56f5d897.png",
                    "goods_title": "Qwen3-72B 限量版纪念T恤"
                },
                "email": null,
                "id": 163,
                "identifier": null,
                "name": "billowing-night-17",
                "phone_number": null,
                "thumbnail": "",
                "blocked": false,
                "type": "contact"
            },
            "assignee": null,
            "team": null,
            "hmac_verified": false
        },
        "status": "pending",
        "custom_attributes": {},
        "snoozed_until": null,
        "unread_count": 1,
        "first_reply_created_at": null,
        "priority": null,
        "waiting_since": 1763177714,
        "agent_last_seen_at": 0,
        "contact_last_seen_at": 1763177714,
        "last_activity_at": 1763177714,
        "timestamp": 1763177714,
        "created_at": 1763177714,
        "updated_at": 1763177714.230474
    },
    "is_private": false
}
```
