package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func logRequest(r *http.Request) {
	log.Printf("[ADMIN-API] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func decodeAdminJSON(body io.Reader, destination any) error {
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra json.RawMessage
	err := decoder.Decode(&extra)
	if err == nil {
		return errors.New("expected a single JSON value")
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func respondAdminJSONError(w http.ResponseWriter, err error) {
	var limitErr *http.MaxBytesError
	if errors.As(err, &limitErr) {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
}

func respondAdminRPCError(w http.ResponseWriter, message string, err error) {
	rpcStatus := status.Convert(err)
	httpStatus := http.StatusInternalServerError
	switch rpcStatus.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition:
		httpStatus = http.StatusBadRequest
	case codes.NotFound:
		httpStatus = http.StatusNotFound
	case codes.AlreadyExists:
		httpStatus = http.StatusConflict
	}
	http.Error(w, message+": "+rpcStatus.Message(), httpStatus)
}

// clientErrorStatus maps validation-ish admin failures to 4xx.
func clientErrorStatus(msg string) int {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"),
		strings.HasPrefix(lower, "backend does not exist in config:"),
		strings.HasPrefix(lower, "virtual host does not exist in config:"):
		return http.StatusNotFound
	case strings.Contains(lower, "already exists"), strings.Contains(lower, "duplicate"), strings.Contains(lower, "conflict"):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
