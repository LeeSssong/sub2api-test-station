package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountProcurementCostRouter(stub *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.PUT("/accounts/:id", handler.Update)
	return router
}

func TestExistingAccountUpdateRoutesProcurementThroughVersionLedger(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions").WithArgs("legacy-edit-1").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
	mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(3)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(1))
	mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(3), 1, 4.0, 120.0, createdAt, int64(42), "legacy-edit-1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE accounts SET procurement_cost_cny").WithArgs(int64(3), 4.0, 120.0, createdAt, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(42), "/admin/accounts/3/procurement", "legacy-edit-1", int64(3), 4.0, 120.0).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	stub := newStubAdminService()
	amount := 4.0
	quota := 120.0
	stub.getAccountResult = &service.Account{ID: 3, Status: service.StatusActive, ProcurementCostCNY: &amount, EstimatedUsableQuotaUSD: &quota}
	h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetProcurementProfitabilityService(service.NewAccountProfitabilityService(db))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.PUT("/accounts/:id", h.Update)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(`{"procurement_cost_cny":4,"estimated_usable_quota_usd":120}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "legacy-edit-1")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Zero(t, stub.updateAccountCalls, "procurement-only PUT must not run the general account update")
	var responseBody struct {
		Data struct {
			ProcurementCostCNY      *float64 `json:"procurement_cost_cny"`
			EstimatedUsableQuotaUSD *float64 `json:"estimated_usable_quota_usd"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, 4.0, *responseBody.Data.ProcurementCostCNY)
	require.Equal(t, 120.0, *responseBody.Data.EstimatedUsableQuotaUSD)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountHandlerProcurementFailureDoesNotRunGeneralAccountUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions").WithArgs("procurement-failure-1").WillReturnError(errors.New("ledger unavailable"))

	stub := newStubAdminService()
	h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetProcurementProfitabilityService(service.NewAccountProfitabilityService(db))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.PUT("/accounts/:id", h.Update)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(`{"procurement_cost_cny":4,"estimated_usable_quota_usd":120}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "procurement-failure-1")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Zero(t, stub.updateAccountCalls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountHandlerUpdateProcurementCostDistinguishesOmittedNumberAndNull(t *testing.T) {
	effectiveAt := time.Date(2026, time.August, 4, 9, 30, 0, 0, time.UTC)
	amount := 12.50
	tests := []struct {
		name          string
		body          string
		wantProvided  bool
		wantNilAmount bool
		wantAmount    float64
	}{
		{name: "omitted preserves existing values", body: `{}`, wantProvided: false},
		{name: "number writes amount", body: `{"procurement_cost_cny":12.50,"estimated_usable_quota_usd":60}`, wantProvided: true, wantAmount: 12.50},
		{name: "null clears both values", body: `{"procurement_cost_cny":null,"estimated_usable_quota_usd":null}`, wantProvided: true, wantNilAmount: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubAdminService()
			stub.updateAccountResult = &service.Account{
				ID:                         3,
				Status:                     service.StatusActive,
				ProcurementCostCNY:         &amount,
				ProcurementCostEffectiveAt: &effectiveAt,
			}
			router := setupAccountProcurementCostRouter(stub)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, 1, stub.updateAccountCalls)
			require.Equal(t, tt.wantProvided, stub.lastUpdateAccountInput.ProcurementCost != nil)
			if tt.wantProvided {
				require.Equal(t, tt.wantNilAmount, stub.lastUpdateAccountInput.ProcurementCost.Value == nil)
				if !tt.wantNilAmount {
					require.Equal(t, tt.wantAmount, *stub.lastUpdateAccountInput.ProcurementCost.Value)
				}
			}

			var responseBody struct {
				Data struct {
					ProcurementCostCNY         *float64   `json:"procurement_cost_cny"`
					ProcurementCostEffectiveAt *time.Time `json:"procurement_cost_effective_at"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
			require.Equal(t, 12.50, *responseBody.Data.ProcurementCostCNY)
			require.Equal(t, effectiveAt, *responseBody.Data.ProcurementCostEffectiveAt)
		})
	}
}

func TestAccountHandlerUpdateProcurementCostRejectsInvalidNumbersBeforeServiceWrite(t *testing.T) {
	for _, body := range []string{
		`{"procurement_cost_cny":-0.01,"estimated_usable_quota_usd":60}`,
		`{"procurement_cost_cny":NaN,"estimated_usable_quota_usd":60}`,
		`{"procurement_cost_cny":1e999,"estimated_usable_quota_usd":60}`,
	} {
		t.Run(body, func(t *testing.T) {
			stub := newStubAdminService()
			router := setupAccountProcurementCostRouter(stub)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, stub.updateAccountCalls)
		})
	}
}

func TestAccountHandlerUpdateProcurementCostRequiresEstimatedQuota(t *testing.T) {
	stub := newStubAdminService()
	router := setupAccountProcurementCostRouter(stub)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(`{"procurement_cost_cny":4,"estimated_usable_quota_usd":null}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, stub.updateAccountCalls)
}

func TestAccountHandlerUpdateProcurementCostPersistsEstimatedQuota(t *testing.T) {
	amount := 4.0
	quota := 120.0
	stub := newStubAdminService()
	stub.updateAccountResult = &service.Account{
		ID:                      3,
		Status:                  service.StatusActive,
		ProcurementCostCNY:      &amount,
		EstimatedUsableQuotaUSD: &quota,
	}
	router := setupAccountProcurementCostRouter(stub)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(`{"procurement_cost_cny":4,"estimated_usable_quota_usd":120}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, stub.lastUpdateAccountInput.ProcurementCost)
	require.Equal(t, 120.0, *stub.lastUpdateAccountInput.ProcurementCost.EstimatedUsableQuotaUSD)

	var responseBody struct {
		Data struct {
			EstimatedUsableQuotaUSD *float64 `json:"estimated_usable_quota_usd"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, 120.0, *responseBody.Data.EstimatedUsableQuotaUSD)
}

func TestAccountHandlerUpdateProcurementCostRejectsInvalidEstimatedQuotaBeforeServiceWrite(t *testing.T) {
	for _, body := range []string{
		`{"procurement_cost_cny":4,"estimated_usable_quota_usd":0}`,
		`{"procurement_cost_cny":4,"estimated_usable_quota_usd":-0.01}`,
		`{"procurement_cost_cny":4,"estimated_usable_quota_usd":NaN}`,
		`{"procurement_cost_cny":4,"estimated_usable_quota_usd":1e999}`,
	} {
		t.Run(body, func(t *testing.T) {
			stub := newStubAdminService()
			router := setupAccountProcurementCostRouter(stub)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, stub.updateAccountCalls)
		})
	}
}

func TestAccountHandlerUpdateRejectsPriorityBelowOneBeforeServiceWrite(t *testing.T) {
	for _, priority := range []int{0, -1} {
		t.Run("priority_"+strconv.Itoa(priority), func(t *testing.T) {
			stub := newStubAdminService()
			router := setupAccountProcurementCostRouter(stub)
			body, err := json.Marshal(map[string]any{"priority": priority})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			var response map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "priority must be >= 1", response["message"])
			require.Zero(t, stub.updateAccountCalls)
		})
	}
}

func TestAccountHandlerBulkUpdateRejectsPriorityBelowOneBeforeServiceWrite(t *testing.T) {
	for _, priority := range []int{0, -1} {
		t.Run("priority_"+strconv.Itoa(priority), func(t *testing.T) {
			stub := newStubAdminService()
			gin.SetMode(gin.TestMode)
			router := gin.New()
			handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router.POST("/accounts/bulk-update", handler.BulkUpdate)
			body, err := json.Marshal(map[string]any{"account_ids": []int64{3}, "priority": priority})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/accounts/bulk-update", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			var response map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "priority must be >= 1", response["message"])
			require.Nil(t, stub.lastBulkUpdateAccountInput)
		})
	}
}

func TestAccountHandlerProcurementFailureReturnsDiagnosticContract(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions").WithArgs("diagnostic-1").WillReturnError(errors.New("ledger unavailable"))

	stub := newStubAdminService()
	h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetProcurementProfitabilityService(service.NewAccountProfitabilityService(db))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.PUT("/accounts/:id", h.Update)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(`{"procurement_cost_cny":4,"estimated_usable_quota_usd":120}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "diagnostic-1")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var body struct {
		Message, Reason string
		Metadata        map[string]string
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "采购成本保存失败，请稍后重试", body.Message)
	require.Equal(t, "procurement_update_failed", body.Reason)
	require.Equal(t, "diagnostic-1", body.Metadata["request_id"])
	require.Equal(t, "3", body.Metadata["account_id"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountHandlerProcurementSavedReadbackFailureIsRecognizableAndReplayWritesOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectBegin()
		if attempt == 0 {
			mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions").WithArgs("readback-1").WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("SELECT created_at FROM accounts").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(createdAt))
			mock.ExpectQuery("SELECT id,cost_cny").WithArgs(int64(3)).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\)\\+1").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"next"}).AddRow(1))
			mock.ExpectExec("INSERT INTO account_procurement_cost_versions").WithArgs(int64(3), 1, 4.0, 120.0, createdAt, int64(42), "readback-1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec("UPDATE accounts SET procurement_cost_cny").WithArgs(int64(3), 4.0, 120.0, createdAt, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO audit_logs").WithArgs(int64(42), "/admin/accounts/3/procurement", "readback-1", int64(3), 4.0, 120.0).WillReturnResult(sqlmock.NewResult(1, 1))
		} else {
			mock.ExpectQuery("SELECT account_id FROM account_procurement_cost_versions").WithArgs("readback-1").WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(3)))
		}
		mock.ExpectCommit()
	}
	stub := newStubAdminService()
	stub.getAccountErr = errors.New("read replica unavailable")
	h := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetProcurementProfitabilityService(service.NewAccountProfitabilityService(db))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.PUT("/accounts/:id", h.Update)
	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/accounts/3", bytes.NewBufferString(`{"procurement_cost_cny":4,"estimated_usable_quota_usd":120}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "readback-1")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
		var body struct {
			Message, Reason string
			Metadata        map[string]string
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, "procurement_saved_readback_failed", body.Reason)
		require.Equal(t, "readback-1", body.Metadata["request_id"])
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
