# GitHub Copilot 平台支持

Sub2API 支持通过 GitHub Copilot 订阅账号提供 AI 能力。用户可以将 GitHub Copilot 账号接入平台，然后通过平台生成的 API Key 使用 Copilot 背后的 GPT、Claude、Gemini 等模型。

---

## 功能概述

- **OAuth 授权**：通过 GitHub Device OAuth 流程授权，无需手动管理 Token
- **多账号调度**：支持多个 Copilot 账号的智能调度和故障转移
- **双协议支持**：
  - OpenAI 兼容接口（`/copilot/v1/chat/completions`）
  - Anthropic 兼容接口（`/copilot/v1/messages`）—— **Claude Code 专用**
- **额度查询**：可在管理后台查看各账号的 Premium 请求额度使用情况
- **用量计费**：按 Token 精确计费，记录到用户用量

---

## 支持的模型

通过 GitHub Copilot 可访问以下模型（取决于订阅计划）：

| 模型 ID | 显示名称 |
|---------|----------|
| `gpt-4o` | GPT-4o |
| `gpt-4o-mini` | GPT-4o Mini |
| `gpt-4.1` | GPT-4.1 |
| `gpt-4.1-mini` | GPT-4.1 Mini |
| `gpt-4.1-nano` | GPT-4.1 Nano |
| `o4-mini` | o4 Mini |
| `o3-mini` | o3 Mini |
| `claude-sonnet-4` | Claude Sonnet 4 |
| `claude-sonnet-4-5` | Claude Sonnet 4.5 |
| `claude-sonnet-4-6` | Claude Sonnet 4.6 |
| `claude-opus-4-5` | Claude Opus 4.5 |
| `claude-opus-4-6` | Claude Opus 4.6 |
| `claude-haiku-4-5` | Claude Haiku 4.5 |
| `claude-3.5-sonnet` | Claude 3.5 Sonnet |
| `gemini-2.0-flash-001` | Gemini 2.0 Flash |

> **注意**：实际可用模型以账号当前订阅计划为准，可调用 `/copilot/v1/models` 获取实时列表。

---

## 管理员：添加 Copilot 账号

### 1. 发起 OAuth 授权

在管理后台 **账号管理** 中选择添加 Copilot 类型账号，系统将调用：

```
POST /api/v1/admin/copilot/oauth/device-code
```

返回：

```json
{
  "session_id": "copilot_1234567890",
  "user_code": "XXXX-XXXX",
  "verification_uri": "https://github.com/login/device",
  "expires_in": 900,
  "interval": 5
}
```

### 2. 用户授权

在浏览器中访问 `https://github.com/login/device`，输入显示的 `user_code`，完成 GitHub 账号授权。

### 3. 轮询确认

系统自动轮询以下接口，等待授权完成：

```
POST /api/v1/admin/copilot/oauth/poll
{ "session_id": "copilot_1234567890" }
```

授权成功后，GitHub Token 自动保存到账号配置，无需手动操作。

### 4. 验证账号

添加后可在账号详情页点击「测试连接」，或查看账号的 Copilot 额度：

```
GET /api/v1/admin/accounts/:id/copilot-quota
```

返回：

```json
{
  "plan": "copilot_enterprise",
  "plan_type": "Enterprise",
  "premium_interactions": {
    "entitlement": 300,
    "used": 42,
    "overage_permitted": false
  },
  "quota_reset_date": "2026-04-01"
}
```

---

## API 端点

所有 Copilot 端点均挂载在 `/copilot/v1` 路径下，需携带平台 API Key 进行认证。

### OpenAI 兼容端点

#### 聊天补全

```
POST /copilot/v1/chat/completions
Authorization: Bearer sk-xxx
Content-Type: application/json
```

请求体与标准 OpenAI Chat Completions API 完全兼容。支持流式（`"stream": true`）和非流式两种模式。

**示例：**

```bash
curl https://your-sub2api-instance.com/copilot/v1/chat/completions \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

#### 模型列表

```
GET /copilot/v1/models
Authorization: Bearer sk-xxx
```

返回当前可用的 Copilot 模型列表（OpenAI 格式）。

---

### Anthropic 兼容端点（Claude Code 专用）

```
POST /copilot/v1/messages
Authorization: Bearer sk-xxx
Content-Type: application/json
```

请求和响应格式与 Anthropic Messages API 完全兼容。平台在内部自动完成格式转换：

```
Claude Code → Anthropic 格式 → [Sub2API 转换] → OpenAI 格式 → GitHub Copilot API
GitHub Copilot API → OpenAI 格式 → [Sub2API 转换] → Anthropic 格式 → Claude Code
```

---

## Claude Code 配置

Claude Code 使用 Anthropic 协议，Sub2API 提供了完整的协议适配层。

### 配置方式

**方式一：环境变量（推荐）**

```bash
export ANTHROPIC_BASE_URL="https://your-sub2api-instance.com/copilot"
export ANTHROPIC_AUTH_TOKEN="sk-xxx"
```

> `ANTHROPIC_BASE_URL` 填写到 `/copilot` 为止（不含 `/v1`），Claude Code 会自动补全后续路径。

**方式二：`claude` 命令行**

```bash
ANTHROPIC_BASE_URL="https://your-sub2api-instance.com/copilot" \
ANTHROPIC_AUTH_TOKEN="sk-xxx" \
claude
```

**方式三：`~/.claude/.env` 文件**

```env
ANTHROPIC_BASE_URL=https://your-sub2api-instance.com/copilot
ANTHROPIC_AUTH_TOKEN=sk-xxx
```

### 使用 ultrahink 实例

如果你使用的是 ultrahink 平台的 Sub2API 服务：

```bash
export ANTHROPIC_BASE_URL="https://ultrahink.com/copilot"
export ANTHROPIC_AUTH_TOKEN="sk-xxx"   # 替换为你的 API Key
```

或写入 `~/.claude/.env`：

```env
ANTHROPIC_BASE_URL=https://ultrahink.com/copilot
ANTHROPIC_AUTH_TOKEN=sk-xxx
```

### 模型选择

Claude Code 支持通过 `--model` 参数指定模型：

```bash
claude --model claude-sonnet-4-5
```

或在 Claude Code 内部使用 `/model` 命令切换。

### 已知问题

**Plan Mode 无法自动退出**

使用 Copilot 账号时，Claude Code 的 Plan Mode 完成规划后不会自动弹出确认选项。

**解决方法**：按 `Shift + Tab` 手动退出 Plan Mode，然后输入回复来批准或拒绝计划。

---

## 工作原理

### Token 管理

Copilot 账号通过两层 Token 体系工作：

1. **GitHub Token**：通过 Device OAuth 获取，长期有效，存储在账号配置中
2. **Copilot API Token**：由 GitHub Token 交换得到，短期有效（约 30 分钟），由平台自动缓存和刷新

Token 交换通过 GitHub 内部接口进行：

```
GET https://api.github.com/copilot_internal/v2/token
Authorization: token <github_token>
```

### 协议转换（Anthropic ↔ OpenAI）

`/copilot/v1/messages` 端点实现了 Anthropic 和 OpenAI 协议之间的双向转换，包括：

- 消息格式转换（system/user/assistant 角色映射）
- 工具调用格式转换（tool_use ↔ function calling）
- 流式响应转换（Anthropic SSE 事件格式 ↔ OpenAI SSE delta 格式）
- Claude 模型 ID 规范化（点分格式 `4.5` ↔ 短横线格式 `4-5`）

### 账号调度

请求到达时，平台根据以下策略选择 Copilot 账号：

- 过滤平台类型为 `copilot` 的账号
- 排除当前请求失败的账号（最多重试 3 次）
- 若所有账号均失败，返回 `502 Bad Gateway`

---

## 常见问题

**Q：如何知道账号的 Premium 额度还剩多少？**

在管理后台账号详情中查看「Copilot 额度」，或调用管理 API 的 `/api/v1/admin/accounts/:id/copilot-quota`。

**Q：Copilot Individual / Business / Enterprise 有什么区别？**

不同计划的 Premium 交互额度不同。Enterprise 通常额度更高，并且使用 `api.business.githubcopilot.com` 作为 API 端点（可在账号配置中自定义 `base_url`）。

**Q：Claude Code 报错 `Invalid API Key`？**

请确认 `ANTHROPIC_AUTH_TOKEN` 中填写的是 Sub2API 平台生成的 API Key（以 `sk-` 开头），而不是 GitHub Token 或 Anthropic 原始 Token。

**Q：为什么模型列表里 Claude 模型 ID 包含短横线而不是点？**

GitHub Copilot API 返回的模型 ID 如 `claude-sonnet-4.5`，但 Claude Code 内置了模型白名单，只接受短横线格式（如 `claude-sonnet-4-5`）。平台在 `/copilot/v1/models` 响应中自动进行了格式转换，发送请求时再反向转换回点分格式。

**Q：能否同时使用 Copilot 和原生 Anthropic 账号？**

可以。使用分组（Group）功能将不同类型的账号隔离。Copilot 请求通过 `/copilot/v1` 前缀路由，原生 Anthropic 请求通过 `/v1` 路由，互不干扰。
