package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx, recorder
}

func TestSuccessWrapsDataEnvelope(t *testing.T) {
	ctx, recorder := newTestContext()
	Success(ctx, http.StatusOK, map[string]int{"accounts": 3})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var envelope struct {
		Data map[string]int `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("malformed success body %q: %v", recorder.Body.String(), err)
	}
	if envelope.Data["accounts"] != 3 {
		t.Fatalf("envelope data = %+v, want accounts:3", envelope.Data)
	}
	if strings.Contains(recorder.Body.String(), "error") {
		t.Fatalf("success envelope leaked error field: %s", recorder.Body.String())
	}
}

func TestErrorSetsStableCodeAndDropsRequestIDWhenAbsent(t *testing.T) {
	ctx, recorder := newTestContext()
	Error(ctx, http.StatusBadRequest, "invalid_input", "参数无效")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("malformed error body %q: %v", recorder.Body.String(), err)
	}
	if envelope.Error.Code != "invalid_input" || envelope.Error.Message != "参数无效" {
		t.Fatalf("envelope error = %+v", envelope.Error)
	}
	if envelope.Error.RequestID != "" {
		t.Fatalf("requestId = %q, want empty", envelope.Error.RequestID)
	}
}

func TestErrorEchoesRequestIDWhenPresent(t *testing.T) {
	ctx, recorder := newTestContext()
	ctx.Set("requestId", "req_123")
	Error(ctx, http.StatusUnauthorized, "unauthorized", "未授权")

	var envelope struct {
		Error struct {
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("malformed error body %q: %v", recorder.Body.String(), err)
	}
	if envelope.Error.RequestID != "req_123" {
		t.Fatalf("requestId = %q, want req_123", envelope.Error.RequestID)
	}
}
