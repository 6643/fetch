# 2026-03-14-fetch-flattening-design

## 1. 背景与目标
本项目旨在对 `fetch` 库进行扁平化重构，严格遵守资深架构师全局约束。核心目标是消除深层嵌套、优化卫语句使用，并提高代码的可读性与健壮性。

## 2. 核心架构准则
- **强制卫语句**：优先处理边缘情况，主逻辑不被 `if` 包裹。
- **极度扁平化**：核心逻辑嵌套严禁超过 2 层。
- **职责拆分**：将复杂逻辑拆分为单一职责的私有函数。

## 3. 详细设计

### 3.1 Request 构建扁平化 (`request.go`)
`buildRequest` 函数目前承担了过多职责，将重构为流式调用：
1.  `validateMethod(method, bodyType)`: 检查方法与 Body 是否冲突。
2.  `buildURL(rawURL, query)`: 处理 URL 解析与 Query 参数合并。
3.  `applyRequestHeaders(req, cfg, contentType)`: 统一处理 Header、Cookie 和 User-Agent。

对于 `prepareMultipartBody`，将协程内的逻辑提取为独立函数，通过 `io.Pipe` 保持流式处理的同时消除闭包嵌套。

### 3.2 Response 处理扁平化 (`response.go`)
`buildResponse` 函数中的循环和判断将拆分为：
1.  `extractHeaders(httpHeader)`: 负责 Header 的克隆与扁平化映射。
2.  `extractCookies(httpResponse)`: 负责解析 Cookie 列表及构建 Cookie 字符串。
3.  `extractLocation(httpResponse)`: 专门处理重定向地址提取。

### 3.3 Transport 配置扁平化 (`transport.go`)
`applyTransportOptions` 将通过一系列独立的配置函数实现，避免复杂的 `if-else` 结构。

## 4. 验证方式
- 运行 `go test ./...` 确保 100% 回归成功。
- 物理检查：代码缩进层级不得超过 2 层。

## 5. 风险与权衡
- **风险**：函数拆分过多可能导致逻辑追踪需要跨越多个函数。
- **对策**：使用清晰的私有函数命名（如 `finalizeURL`, `attachHeaders`）来显式表达意图。
