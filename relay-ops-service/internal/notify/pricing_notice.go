package notify

import (
	"strings"
	"time"
)

type PricingNoticeView struct {
	Upstream   string
	Change     string
	Review     string
	ObservedAt time.Time
}

func RenderPricingNotice(view PricingNoticeView) FeishuMessage {
	upstream := humanText(view.Upstream, "生产上游")
	elements := impactSections([]section{
		{
			Title: "发生了什么",
			Lines: []string{humanText(view.Change, "公开定价出现可靠变化。")},
		},
		{
			Title: "系统未做",
			Lines: []string{"仅记录了公开定价证据；没有修改价格、倍率、路由或账号。"},
		},
		{
			Title: "需要核对",
			Lines: []string{humanText(view.Review, "核对当前售价和毛利是否仍符合预期。")},
		},
		{
			Title: "证据时间",
			Lines: []string{formatObservedAt(view.ObservedAt)},
		},
	})
	elements = append(elements, operationsAction())
	return FeishuMessage{
		MsgType:  "interactive",
		Severity: "P2",
		Card: &Card{
			Config: CardConfig{WideScreenMode: true},
			Header: CardHeader{
				Title: CardText{
					Tag:     "plain_text",
					Content: "价格变更｜" + strings.TrimSpace(upstream) + " 公开定价发生变化",
				},
				Template: "blue",
			},
			Elements: elements,
		},
	}
}
