package domain

type ExecutionHistoryPage struct {
	Items []ExecutionSnapshot `json:"items"`
	Next  string              `json:"next,omitempty"`
}
