package projection

type Accounting struct {
	Metadata Metadata `json:"metadata"`
	Requests int64    `json:"requests"`
	Revenue  string   `json:"revenue"`
	Cost     string   `json:"cost"`
}
