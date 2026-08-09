package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountMonitorHandler struct {
	monitorService     *service.AccountMonitorService
	runner             *service.AccountMonitorRunner
	accountRepo        AccountMonitorConcurrencyAccountRepository
	concurrencyService *service.ConcurrencyService
	runtimeService     accountModelRuntimeService
}

type accountModelRuntimeService interface {
	SnapshotOpenAIAccountModelRuntime(time.Time) []service.OpenAIAccountModelRuntimeSnapshot
	ImmediatelyCooldownAccountModel(context.Context, int64, string, time.Duration, time.Time) (service.OpenAIAccountModelRuntimeSnapshot, error)
	RestoreAccountModelScheduling(context.Context, int64, string) error
	ProbeAccountModelOnce(context.Context, int64, string, time.Time) (bool, error)
	ReleaseOpenAIAccountModelHalfOpenProbe(int64, string, bool, time.Time)
}

type AccountMonitorConcurrencyAccountRepository interface {
	GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error)
}

// SetOpenAIGatewayService attaches the existing gateway runtime owner without
// creating a parallel account-model state store in the admin surface.
func (h *AccountMonitorHandler) SetOpenAIGatewayService(runtime accountModelRuntimeService) {
	if h != nil {
		h.runtimeService = runtime
	}
}

func NewAccountMonitorHandler(
	monitorService *service.AccountMonitorService,
	runner *service.AccountMonitorRunner,
	accountRepo AccountMonitorConcurrencyAccountRepository,
	concurrencyService *service.ConcurrencyService,
) *AccountMonitorHandler {
	return &AccountMonitorHandler{
		monitorService:     monitorService,
		runner:             runner,
		accountRepo:        accountRepo,
		concurrencyService: concurrencyService,
	}
}

const accountMonitorConcurrencyMaxUniqueIDs = 200
const accountModelRuntimeMaxCooldownSeconds = 3600

type accountMonitorConcurrencyRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

type accountMonitorConcurrencyItem struct {
	AccountID int64 `json:"account_id"`
	Current   int   `json:"current"`
	Limit     int   `json:"limit"`
}

type accountMonitorConcurrencyResponse struct {
	Items []accountMonitorConcurrencyItem `json:"items"`
}

type accountMonitorSettingsRequest struct {
	IntervalSeconds int `json:"interval_seconds" binding:"required"`
}

type accountMonitorHistoryResponse struct {
	Items []dto.AccountMonitorHistoryItem `json:"items"`
}

type accountModelRuntimeActionRequest struct {
	AccountID                int64  `json:"account_id" binding:"required"`
	CanonicalSchedulingModel string `json:"canonical_scheduling_model" binding:"required"`
	CooldownSeconds          int    `json:"cooldown_seconds"`
}

type accountModelRuntimeResponse struct {
	Items []dto.AccountModelRuntimeItem `json:"items"`
}

type accountMonitorScoreWeightsRequest struct {
	Cost            int  `json:"cost"`
	Success         int  `json:"success"`
	TTFT            int  `json:"ttft"`
	Latency         int  `json:"latency"`
	TTFTTargetMS    *int `json:"ttft_target_ms"`
	TTFTLimitMS     *int `json:"ttft_limit_ms"`
	LatencyTargetMS *int `json:"latency_target_ms"`
	LatencyLimitMS  *int `json:"latency_limit_ms"`
}

func (h *AccountMonitorHandler) List(c *gin.Context) {
	rawRange := c.Query("range")
	if _, _, err := service.ParseAccountMonitorRange(rawRange); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_MONITOR_RANGE", err.Error()))
		return
	}
	page, err := h.monitorService.ListWindow(c.Request.Context(), rawRange)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, page)
}

func (h *AccountMonitorHandler) ListAccountModelRuntime(c *gin.Context) {
	if h == nil || h.runtimeService == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_MODEL_RUNTIME_UNAVAILABLE", "OpenAI account model runtime is unavailable"))
		return
	}
	response.Success(c, accountModelRuntimeResponse{Items: accountModelRuntimeDTOs(h.runtimeService.SnapshotOpenAIAccountModelRuntime(time.Now().UTC()))})
}

func (h *AccountMonitorHandler) ImmediatelyCooldownAccountModel(c *gin.Context) {
	var req accountModelRuntimeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AccountID <= 0 || strings.TrimSpace(req.CanonicalSchedulingModel) == "" || req.CooldownSeconds <= 0 || req.CooldownSeconds > accountModelRuntimeMaxCooldownSeconds {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_MODEL_RUNTIME_ACTION", "account_id, canonical_scheduling_model and cooldown_seconds are required"))
		return
	}
	if h == nil || h.runtimeService == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_MODEL_RUNTIME_UNAVAILABLE", "OpenAI account model runtime is unavailable"))
		return
	}
	item, err := h.runtimeService.ImmediatelyCooldownAccountModel(c.Request.Context(), req.AccountID, req.CanonicalSchedulingModel, time.Duration(req.CooldownSeconds)*time.Second, time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_MODEL_RUNTIME_ACTION_FAILED", err.Error()))
		return
	}
	response.Success(c, accountModelRuntimeItemDTO(item))
}

func (h *AccountMonitorHandler) RestoreAccountModelScheduling(c *gin.Context) {
	var req accountModelRuntimeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AccountID <= 0 || strings.TrimSpace(req.CanonicalSchedulingModel) == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_MODEL_RUNTIME_ACTION", "account_id and canonical_scheduling_model are required"))
		return
	}
	if h == nil || h.runtimeService == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_MODEL_RUNTIME_UNAVAILABLE", "OpenAI account model runtime is unavailable"))
		return
	}
	if err := h.runtimeService.RestoreAccountModelScheduling(c.Request.Context(), req.AccountID, req.CanonicalSchedulingModel); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_MODEL_RUNTIME_ACTION_FAILED", err.Error()))
		return
	}
	response.Success(c, gin.H{"restored": true})
}

func (h *AccountMonitorHandler) ProbeAccountModelOnce(c *gin.Context) {
	var req accountModelRuntimeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AccountID <= 0 || strings.TrimSpace(req.CanonicalSchedulingModel) == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_MODEL_RUNTIME_ACTION", "account_id and canonical_scheduling_model are required"))
		return
	}
	if h == nil || h.runtimeService == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_MODEL_RUNTIME_UNAVAILABLE", "OpenAI account model runtime is unavailable"))
		return
	}
	pending, err := h.runtimeService.ProbeAccountModelOnce(c.Request.Context(), req.AccountID, req.CanonicalSchedulingModel, time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_MODEL_RUNTIME_ACTION_FAILED", err.Error()))
		return
	}
	if !pending {
		response.Success(c, gin.H{"pending": false, "probed": false})
		return
	}
	if h.monitorService == nil {
		h.runtimeService.ReleaseOpenAIAccountModelHalfOpenProbe(req.AccountID, req.CanonicalSchedulingModel, false, time.Now().UTC())
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_MODEL_PROBE_UNAVAILABLE", "account monitor probe is unavailable"))
		return
	}
	success, probeErr := h.monitorService.ProbeOpenAIAccountModel(c.Request.Context(), req.AccountID, req.CanonicalSchedulingModel)
	h.runtimeService.ReleaseOpenAIAccountModelHalfOpenProbe(req.AccountID, req.CanonicalSchedulingModel, success, time.Now().UTC())
	if probeErr != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_MODEL_PROBE_FAILED", probeErr.Error()))
		return
	}
	response.Success(c, gin.H{"pending": false, "probed": true, "success": success})
}

func accountModelRuntimeDTOs(snapshots []service.OpenAIAccountModelRuntimeSnapshot) []dto.AccountModelRuntimeItem {
	items := make([]dto.AccountModelRuntimeItem, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, accountModelRuntimeItemDTO(snapshot))
	}
	return items
}

func accountModelRuntimeItemDTO(snapshot service.OpenAIAccountModelRuntimeSnapshot) dto.AccountModelRuntimeItem {
	item := dto.AccountModelRuntimeItem{AccountID: snapshot.AccountID, CanonicalSchedulingModel: snapshot.CanonicalModel, State: snapshot.State, FailureStreak: snapshot.FailureStreak, HalfOpenInFlight: snapshot.HalfOpenInFlight, LastStatusCode: snapshot.LastStatusCode, LastErrorType: snapshot.LastErrorType, OutputStarted: snapshot.OutputStarted, StickyReferenceCount: snapshot.StickyReferenceCount}
	if !snapshot.LastFailureAt.IsZero() {
		value := snapshot.LastFailureAt.UTC().Format(time.RFC3339)
		item.LastFailureAt = &value
	}
	if !snapshot.BlockUntil.IsZero() {
		value := snapshot.BlockUntil.UTC().Format(time.RFC3339)
		item.CooldownUntil = &value
	}
	return item
}

func (h *AccountMonitorHandler) Concurrency(c *gin.Context) {
	var req accountMonitorConcurrencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}

	accountIDs := make([]int64, 0, len(req.AccountIDs))
	seen := make(map[int64]struct{}, len(req.AccountIDs))
	for _, accountID := range req.AccountIDs {
		if accountID <= 0 {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "account_ids must contain only positive IDs"))
			return
		}
		if _, ok := seen[accountID]; ok {
			continue
		}
		seen[accountID] = struct{}{}
		accountIDs = append(accountIDs, accountID)
		if len(accountIDs) > accountMonitorConcurrencyMaxUniqueIDs {
			response.ErrorFrom(c, infraerrors.BadRequest("TOO_MANY_ACCOUNT_IDS", "account_ids must contain at most 200 unique IDs"))
			return
		}
	}
	if len(accountIDs) == 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_IDS_REQUIRED", "account_ids must not be empty"))
		return
	}
	if h.accountRepo == nil || h.concurrencyService == nil {
		response.ErrorFrom(c, infraerrors.InternalServer("ACCOUNT_MONITOR_CONCURRENCY_UNAVAILABLE", "account concurrency is unavailable"))
		return
	}

	accounts, err := h.accountRepo.GetByIDs(c.Request.Context(), accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountsByID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountsByID[account.ID] = account
		}
	}
	for _, accountID := range accountIDs {
		if _, ok := accountsByID[accountID]; !ok {
			response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_NOT_VISIBLE", "one or more accounts are not visible"))
			return
		}
	}

	currentByID, err := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]accountMonitorConcurrencyItem, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		items = append(items, accountMonitorConcurrencyItem{
			AccountID: accountID,
			Current:   currentByID[accountID],
			Limit:     accountsByID[accountID].Concurrency,
		})
	}
	response.Success(c, accountMonitorConcurrencyResponse{Items: items})
}

func (h *AccountMonitorHandler) UpdateSettings(c *gin.Context) {
	var req accountMonitorSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	settings, err := h.monitorService.UpdateSettings(
		c.Request.Context(),
		subject.UserID,
		req.IntervalSeconds,
	)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_INTERVAL", err.Error()))
		return
	}
	h.runner.ReloadSettings(settings)
	response.Success(c, settings)
}

func (h *AccountMonitorHandler) Run(c *gin.Context) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	completed, err := h.monitorService.RunAll(c.Request.Context(), subject.UserID)
	if err != nil {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	response.Success(c, gin.H{"completed": completed})
}

func (h *AccountMonitorHandler) RunOne(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "invalid account id"))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	result, err := h.monitorService.RunOne(c.Request.Context(), subject.UserID, accountID)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("ACCOUNT_MONITOR_RUN_FAILED", err.Error()))
		return
	}
	response.Success(c, result)
}

func (h *AccountMonitorHandler) History(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "invalid account id"))
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		limit, _ = strconv.Atoi(raw)
	}
	history, err := h.monitorService.History(c.Request.Context(), accountID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]dto.AccountMonitorHistoryItem, 0, len(history))
	for _, row := range history {
		items = append(items, dto.AccountMonitorHistoryItem{
			AccountID:  row.AccountID,
			ModelID:    row.ModelID,
			Status:     row.Status,
			ErrorCode:  row.ErrorCode,
			HTTPStatus: row.HTTPStatus,
			TTFTMS:     row.TTFTMS,
			LatencyMS:  row.LatencyMS,
			CheckedAt:  row.CheckedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	response.Success(c, accountMonitorHistoryResponse{Items: items})
}

func (h *AccountMonitorHandler) GetGroupScoreWeights(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	weights, err := h.monitorService.GetGroupScoreWeights(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}

func (h *AccountMonitorHandler) UpdateGroupScoreWeights(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	var req accountMonitorScoreWeightsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	current, err := h.monitorService.GetGroupScoreWeights(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	weights := service.AccountMonitorScoreWeights{
		Cost: req.Cost, Success: req.Success, TTFT: req.TTFT, Latency: req.Latency,
		TTFTTargetMS: current.TTFTTargetMS, TTFTLimitMS: current.TTFTLimitMS,
		LatencyTargetMS: current.LatencyTargetMS, LatencyLimitMS: current.LatencyLimitMS,
	}
	if req.TTFTTargetMS != nil {
		weights.TTFTTargetMS = *req.TTFTTargetMS
	}
	if req.TTFTLimitMS != nil {
		weights.TTFTLimitMS = *req.TTFTLimitMS
	}
	if req.LatencyTargetMS != nil {
		weights.LatencyTargetMS = *req.LatencyTargetMS
	}
	if req.LatencyLimitMS != nil {
		weights.LatencyLimitMS = *req.LatencyLimitMS
	}
	weights, err = h.monitorService.UpdateGroupScoreWeights(c.Request.Context(), groupID, subject.UserID, weights)
	if err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_SCORE_WEIGHTS", err.Error()))
		return
	}
	response.Success(c, weights)
}

func (h *AccountMonitorHandler) ResetGroupScoreWeights(c *gin.Context) {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group id"))
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	weights, err := h.monitorService.ResetGroupScoreWeights(c.Request.Context(), groupID, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, weights)
}
