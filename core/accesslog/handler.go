package accesslog

import (
	"bytes"
	"io"
	"strings"

	"github.com/huwenlong92/sdkit/core/jsonx"

	"github.com/gin-gonic/gin"
)

// GetRequestBody 提取请求体，按 Content-Type 解析为 map
func GetRequestBody(c *gin.Context) (map[string]interface{}, error) {
	const defaultMemory = 32 << 20
	contentType := c.Request.Header.Get("Content-Type")

	postMap := make(map[string]interface{})

	if strings.Contains(contentType, "application/json") {
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		if len(bodyBytes) > 0 {
			if err := jsonx.Unmarshal(bodyBytes, &postMap); err != nil {
				return nil, err
			}
		}
		return postMap, nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(defaultMemory); err != nil {
			return nil, err
		}
	} else {
		if err := c.Request.ParseForm(); err != nil {
			return nil, err
		}
		// ParseMultipartForm 在非 multipart 时必报错，忽略即可
		_ = c.Request.ParseMultipartForm(defaultMemory)
	}

	for k, v := range c.Request.PostForm {
		if len(v) > 1 {
			postMap[k] = v
		} else if len(v) == 1 {
			postMap[k] = v[0]
		}
	}
	return postMap, nil
}

// GetRequestQuery 提取 URL 查询参数为 map
func GetRequestQuery(c *gin.Context) map[string]interface{} {
	queryMap := make(map[string]interface{})
	for k := range c.Request.URL.Query() {
		queryMap[k] = c.Query(k)
	}
	return queryMap
}

// GetRequestHeaders 提取请求头为 map
func GetRequestHeaders(c *gin.Context) map[string]interface{} {
	headers := make(map[string]interface{})
	for k := range c.Request.Header {
		headers[k] = c.GetHeader(k)
	}
	return headers
}

// RequestInputs 分离 GET 查询参数和 POST 请求体
func RequestInputs(c *gin.Context) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"GET": GetRequestQuery(c),
	}

	if c.Request.Method == "POST" {
		postMap, err := GetRequestBody(c)
		if err != nil {
			return nil, err
		}
		result["POST"] = postMap
	}

	return result, nil
}
