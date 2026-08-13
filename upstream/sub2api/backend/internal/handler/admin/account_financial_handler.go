package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountFinancialHandler struct {
	service *service.AccountFinancialService
}

func NewAccountFinancialHandler(s *service.AccountFinancialService) *AccountFinancialHandler {
	return &AccountFinancialHandler{service: s}
}

func (h *AccountFinancialHandler) GetReport(c *gin.Context) {
	r := service.AccountFinancialRange(strings.TrimSpace(c.DefaultQuery("range", "today")))
	if r != service.AccountFinancialRangeToday && r != service.AccountFinancialRange24H && r != service.AccountFinancialRange7D && r != service.AccountFinancialRange31D {
		response.BadRequest(c, "Invalid range")
		return
	}
	if h.service == nil {
		response.InternalError(c, "account financial service unavailable")
		return
	}
	v, err := h.service.GetReport(c.Request.Context(), r)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}

func parseMoney(raw *float64) error { return service.ValidateFinancialAmount(raw) }
func actorID(c *gin.Context) int64 {
	if s, ok := middleware.GetAuthSubjectFromContext(c); ok {
		return s.UserID
	}
	return 0
}

// financialRequestID returns the server correlation ID injected by the HTTP
// middleware. The client ID is a fallback for tests or non-standard mounts;
// the header fallback keeps the audit trail useful when middleware is omitted.
func financialRequestID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	ctx := c.Request.Context()
	if id, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if id, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return strings.TrimSpace(c.GetHeader("X-Request-ID"))
}

func (h *AccountFinancialHandler) ListExceptions(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "account financial service unavailable")
		return
	}
	page, size := response.ParsePagination(c)
	f := service.ReviewFilter{Page: page, PageSize: size, Search: strings.TrimSpace(c.Query("search")), EvidenceStatus: strings.TrimSpace(c.Query("evidence_status")), ReviewStatus: strings.TrimSpace(c.Query("review_status"))}
	if raw := strings.TrimSpace(c.Query("account_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid account_id")
			return
		}
		f.AccountID = &id
	}
	v, err := h.service.ListExceptions(c.Request.Context(), f)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}

type reviewRequest struct {
	ManualCostCNY *float64 `json:"manual_cost_cny"`
}

func (h *AccountFinancialHandler) ReviewOne(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("usageLogID"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid usage log ID")
		return
	}
	var in reviewRequest
	if c.ShouldBindJSON(&in) != nil || parseMoney(in.ManualCostCNY) != nil {
		response.BadRequest(c, "Invalid manual_cost_cny")
		return
	}
	v, err := h.service.ReviewOne(c.Request.Context(), service.UsageCostReviewInput{UsageLogID: id, ManualCostCNY: in.ManualCostCNY, ReviewedBy: actorID(c), ReviewedAt: time.Now(), RequestID: financialRequestID(c)})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}

type selectedRequest struct {
	UsageLogIDs   []int64  `json:"usage_log_ids"`
	ManualCostCNY *float64 `json:"manual_cost_cny"`
}

func (h *AccountFinancialHandler) ReviewSelected(c *gin.Context) {
	var in selectedRequest
	if c.ShouldBindJSON(&in) != nil || len(in.UsageLogIDs) == 0 || parseMoney(in.ManualCostCNY) != nil {
		response.BadRequest(c, "Invalid review request")
		return
	}
	rows := make([]service.UsageCostReviewInput, 0, len(in.UsageLogIDs))
	for _, id := range in.UsageLogIDs {
		if id <= 0 {
			response.BadRequest(c, "Invalid usage log ID")
			return
		}
		rows = append(rows, service.UsageCostReviewInput{UsageLogID: id, ManualCostCNY: in.ManualCostCNY, ReviewedBy: actorID(c), ReviewedAt: time.Now(), RequestID: financialRequestID(c)})
	}
	v, err := h.service.ReviewSelected(c.Request.Context(), rows)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}

type filteredRequest struct {
	Filter        service.ReviewFilter `json:"filter"`
	MaxUsageLogID int64                `json:"max_usage_log_id"`
	ManualCostCNY *float64             `json:"manual_cost_cny"`
}

func (h *AccountFinancialHandler) ReviewFiltered(c *gin.Context) {
	var in filteredRequest
	if c.ShouldBindJSON(&in) != nil || parseMoney(in.ManualCostCNY) != nil {
		response.BadRequest(c, "Invalid review request")
		return
	}
	v, err := h.service.ReviewFiltered(c.Request.Context(), service.ReviewFilteredInput{Filter: in.Filter, MaxUsageLogID: in.MaxUsageLogID, ManualCostCNY: in.ManualCostCNY, ReviewedBy: actorID(c), ReviewedAt: time.Now(), RequestID: financialRequestID(c)})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}

type oauthCostRequest struct {
	BusinessDate string   `json:"business_date"`
	CostCNY      *float64 `json:"cost_cny"`
}

func (h *AccountFinancialHandler) SetOAuthCost(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var in oauthCostRequest
	if c.ShouldBindJSON(&in) != nil || len(in.BusinessDate) != 10 || parseMoney(in.CostCNY) != nil {
		response.BadRequest(c, "Invalid OAuth cost")
		return
	}
	v, err := h.service.SetOAuthDailyCost(c.Request.Context(), service.OAuthDailyCostInput{AccountID: id, BusinessDate: in.BusinessDate, CostCNY: in.CostCNY, ActorUserID: actorID(c), RequestID: financialRequestID(c)})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}

type overrideRequest struct {
	BusinessDate string   `json:"business_date"`
	RevenueCNY   *float64 `json:"revenue_cny"`
	CostCNY      *float64 `json:"cost_cny"`
}

func (h *AccountFinancialHandler) SetTodayOverride(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var in overrideRequest
	if c.ShouldBindJSON(&in) != nil || len(in.BusinessDate) != 10 || parseMoney(in.RevenueCNY) != nil || parseMoney(in.CostCNY) != nil || (in.RevenueCNY == nil) == (in.CostCNY == nil) {
		response.BadRequest(c, "Invalid today override")
		return
	}
	v, err := h.service.SetTodayOverride(c.Request.Context(), service.TodayOverrideInput{AccountID: id, BusinessDate: in.BusinessDate, RevenueCNY: in.RevenueCNY, CostCNY: in.CostCNY, ActorUserID: actorID(c), RequestID: financialRequestID(c)})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, v)
}
