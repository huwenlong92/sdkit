// Package form 提供可复用的表单参数结构体，嵌入 handler 的匿名 request struct 使用
package form

// PageRequest 分页参数
type PageRequest struct {
	Page    int `json:"page" form:"page" binding:"required,min=1"`
	PerPage int `json:"per_page" form:"per_page" binding:"required,min=1,max=100"`
}

// IDRequest 单条 ID 参数
type IDRequest struct {
	ID uint `json:"id" form:"id" binding:"required"`
}

// SortRequest 排序参数
type SortRequest struct {
	OrderColumn string `json:"order_column" form:"order_column"`
	SortType    string `json:"sort_type" form:"sort_type" binding:"omitempty,oneof=asc desc"`
}
