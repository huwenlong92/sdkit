//go:build sdkit_payment_alipay

package openapi

type bizContent struct {
	OutTradeNo     string `json:"out_trade_no"`
	TotalAmount    string `json:"total_amount"`
	Subject        string `json:"subject"`
	Body           string `json:"body,omitempty"`
	ProductCode    string `json:"product_code"`
	TimeoutExpress string `json:"timeout_express,omitempty"`
}

type queryBizContent struct {
	OutTradeNo string `json:"out_trade_no,omitempty"`
	TradeNo    string `json:"trade_no,omitempty"`
}

type refundBizContent struct {
	OutTradeNo   string `json:"out_trade_no,omitempty"`
	TradeNo      string `json:"trade_no,omitempty"`
	RefundAmount string `json:"refund_amount"`
	RefundReason string `json:"refund_reason,omitempty"`
	OutRequestNo string `json:"out_request_no,omitempty"`
	OperatorID   string `json:"operator_id,omitempty"`
	StoreID      string `json:"store_id,omitempty"`
	TerminalID   string `json:"terminal_id,omitempty"`
}

type refundQueryBizContent struct {
	OutTradeNo   string `json:"out_trade_no,omitempty"`
	TradeNo      string `json:"trade_no,omitempty"`
	OutRequestNo string `json:"out_request_no"`
}

type queryGatewayResponse struct {
	Response queryResponse `json:"alipay_trade_query_response"`
	Sign     string        `json:"sign,omitempty"`
}

type refundGatewayResponse struct {
	Response refundResponse `json:"alipay_trade_refund_response"`
	Sign     string         `json:"sign,omitempty"`
}

type refundQueryGatewayResponse struct {
	Response refundQueryResponse `json:"alipay_trade_fastpay_refund_query_response"`
	Sign     string              `json:"sign,omitempty"`
}

type queryResponse struct {
	Code           string `json:"code"`
	Msg            string `json:"msg"`
	SubCode        string `json:"sub_code,omitempty"`
	SubMsg         string `json:"sub_msg,omitempty"`
	TradeNo        string `json:"trade_no,omitempty"`
	OutTradeNo     string `json:"out_trade_no,omitempty"`
	TradeStatus    string `json:"trade_status,omitempty"`
	TotalAmount    string `json:"total_amount,omitempty"`
	ReceiptAmount  string `json:"receipt_amount,omitempty"`
	BuyerPayAmount string `json:"buyer_pay_amount,omitempty"`
	SendPayDate    string `json:"send_pay_date,omitempty"`
}

type refundResponse struct {
	Code         string `json:"code"`
	Msg          string `json:"msg"`
	SubCode      string `json:"sub_code,omitempty"`
	SubMsg       string `json:"sub_msg,omitempty"`
	TradeNo      string `json:"trade_no,omitempty"`
	OutTradeNo   string `json:"out_trade_no,omitempty"`
	BuyerLogonID string `json:"buyer_logon_id,omitempty"`
	FundChange   string `json:"fund_change,omitempty"`
	RefundFee    string `json:"refund_fee,omitempty"`
	SendBackFee  string `json:"send_back_fee,omitempty"`
}

type refundQueryResponse struct {
	Code         string `json:"code"`
	Msg          string `json:"msg"`
	SubCode      string `json:"sub_code,omitempty"`
	SubMsg       string `json:"sub_msg,omitempty"`
	TradeNo      string `json:"trade_no,omitempty"`
	OutTradeNo   string `json:"out_trade_no,omitempty"`
	OutRequestNo string `json:"out_request_no,omitempty"`
	RefundAmount string `json:"refund_amount,omitempty"`
	TotalAmount  string `json:"total_amount,omitempty"`
	RefundStatus string `json:"refund_status,omitempty"`
}
