# Fetch Library Flattening Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the `fetch` library to comply with senior architect flattening rules, eliminating deep nesting and ensuring guard clauses.

**Architecture:** Decompose complex functions in `request.go`, `response.go`, and `transport.go` into focused, single-responsibility private functions with nesting depth ≤ 2.

**Tech Stack:** Go (Standard Library)

---

## Chunk 1: Response Processing Flattening

### Task 1: Refactor `buildResponse` in `response.go`

**Files:**
- Modify: `/._/lib/go/fetch/response.go`

- [ ] **Step 1: Extract Header processing into `extractHeaders`**
- [ ] **Step 2: Extract Cookie processing into `extractCookies`**
- [ ] **Step 3: Extract Location processing into `extractLocation`**
- [ ] **Step 4: Update `buildResponse` to use new helpers**

```go
func buildResponse(httpResponse *http.Response, body []byte) *Response {
	res := &Response{
		StatusCode: httpResponse.StatusCode,
		Status:     httpResponse.Status,
		Body:       body,
		Header:     make(map[string]string),
		Headers:    httpResponse.Header.Clone(),
		Cookie:     make(map[string]string),
	}

	extractHeaders(res, httpResponse.Header)
	extractLocation(res, httpResponse)
	extractCookies(res, httpResponse.Cookies())

	return res
}
```

- [ ] **Step 5: Run tests to verify no regressions**
Run: `unset GOROOT && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**
```bash
git add response.go
git commit -m "refactor: flatten buildResponse in response.go"
```

---

## Chunk 2: Request Building Flattening

### Task 2: Refactor `buildRequest` in `request.go`

**Files:**
- Modify: `/._/lib/go/fetch/request.go`

- [ ] **Step 1: Create `finalizeURL` and `validateMethod` helpers**
- [ ] **Step 2: Create `applyMetadata` helper for Headers, Cookies, and UA**
- [ ] **Step 3: Flatten `buildRequest` logic**

```go
func buildRequest(method, rawURL string, cfg *callConfig) (*http.Request, error) {
	if err := validateMethod(method, cfg.bodySetType); err != nil {
		return nil, err
	}

	parsedURL, err := finalizeURL(rawURL, cfg.query)
	if err != nil {
		return nil, err
	}

	reqBody, contentType, err := prepareBody(cfg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(cfg.ctx, method, parsedURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	applyMetadata(req, cfg, contentType)
	return req, nil
}
```

- [ ] **Step 4: Run tests to verify no regressions**
Run: `unset GOROOT && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add request.go
git commit -m "refactor: flatten buildRequest in request.go"
```

### Task 3: Refactor `prepareMultipartBody` in `request.go`

- [ ] **Step 1: Extract multipart writing logic from closure to `writeMultipartContent`**
- [ ] **Step 2: Simplify `prepareMultipartBody`**

- [ ] **Step 3: Run tests to verify no regressions**
Run: `unset GOROOT && go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**
```bash
git add request.go
git commit -m "refactor: flatten prepareMultipartBody in request.go"
```

---

## Chunk 3: Transport and Core Flow Flattening

### Task 4: Refactor `transport.go` and `do.go`

**Files:**
- Modify: `/._/lib/go/fetch/transport.go`
- Modify: `/._/lib/go/fetch/do.go`

- [ ] **Step 1: Flatten `applyTransportOptions` in `transport.go`**
- [ ] **Step 2: Run tests**
- [ ] **Step 3: Commit transport changes**

- [ ] **Step 4: Run final full test suite**
Run: `unset GOROOT && go test ./...`
Expected: PASS

- [ ] **Step 5: Final Commit**
```bash
git commit -m "refactor: complete flattening of transport and core flow"
```
