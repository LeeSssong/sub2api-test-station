package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		{name: "number writes amount", body: `{"procurement_cost_cny":12.50}`, wantProvided: true, wantAmount: 12.50},
		{name: "null clears both values", body: `{"procurement_cost_cny":null}`, wantProvided: true, wantNilAmount: true},
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
		`{"procurement_cost_cny":-0.01}`,
		`{"procurement_cost_cny":NaN}`,
		`{"procurement_cost_cny":1e999}`,
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
