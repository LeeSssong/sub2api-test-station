package projection

type Reconciliation struct {
	Metadata   Metadata `json:"metadata"`
	Matched    int64    `json:"matched"`
	Exceptions int64    `json:"exceptions"`
}
