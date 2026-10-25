// Package httpx JSON yanıt, tutarlı hata biçimi ve istek gövdesi çözümleme yardımcılarıdır.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// Hata kodları API sözleşmesinin parçasıdır (openapi.yaml).
const (
	CodeValidation         = "VALIDATION_ERROR"
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeInvalidRefresh     = "INVALID_REFRESH_TOKEN"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeInternal           = "INTERNAL_ERROR"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Error("yanıt yazılamadı", "err", err)
		}
	}
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message}})
}

func ValidationError(w http.ResponseWriter, details map[string]string) {
	JSON(w, http.StatusBadRequest, ErrorBody{Error: ErrorDetail{
		Code: CodeValidation, Message: "istek doğrulanamadı", Details: details,
	}})
}

// Internal ayrıntıyı loglar, istemciye genel mesaj döner.
func Internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "iç hata", "err", err, "path", r.URL.Path)
	Error(w, http.StatusInternalServerError, CodeInternal, "beklenmeyen bir hata oluştu")
}

const maxBody = 1 << 20

// Decode gövdeyi dst içine çözer; bilinmeyen alanları ve fazla veriyi reddeder.
func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		msg := "geçersiz JSON gövdesi"
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			msg = "istek gövdesi çok büyük"
		}
		Error(w, http.StatusBadRequest, CodeValidation, msg)
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		Error(w, http.StatusBadRequest, CodeValidation, "gövdede tek JSON nesnesi olmalı")
		return false
	}
	return true
}

// AppError iş/doğrulama hatasıdır; Fail bunu ilgili HTTP yanıtına çevirir.
type AppError struct {
	Status  int
	Code    string
	Message string
	Details map[string]string
}

func (e *AppError) Error() string { return e.Code + ": " + e.Message }

func NewError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

// NotFound kapsam dışı veya olmayan kayıt (başkasına ait kaydın varlığı sızdırılmaz).
func NotFound(what string) *AppError {
	return NewError(http.StatusNotFound, CodeNotFound, what+" bulunamadı")
}

func Invalid(details map[string]string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Code: CodeValidation, Message: "istek doğrulanamadı", Details: details}
}

// Fail AppError ise kendi durum koduyla, değilse 500 olarak yazar.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		JSON(w, ae.Status, ErrorBody{Error: ErrorDetail{Code: ae.Code, Message: ae.Message, Details: ae.Details}})
		return
	}
	Internal(w, r, err)
}
