package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"boten.ai/boten/models"
	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenTTLSeconds = 60 * 60 * 24 * 30 // 30 days
)

type RHandler struct {
	Client *firestore.Client
}

func NewRHandler(client *firestore.Client) *RHandler {
	return &RHandler{Client: client}
}

type ChessMoveRequest struct {
	Board          string   `json:"board"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	AvailableMoves []string `json:"availableMoves"`
	InvalidMoves   []string `json:"invalidMoves"`
}

type ChessMoveResponse struct {
	Move   string `json:"move"`
	Reason string `json:"reason"`
}

type UserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	Email   string  `json:"email"`
	Credits float64 `json:"credits"`
	Token   string  `json:"token,omitempty"` // Optional token for signin
}

func (h *RHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var status int
	w.Header().Set("Content-Type", "application/json")
	// Allow CORS.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin, Accept, Authorization, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")
	defer func() {
		log.Println(r.Method, r.URL.Path, status, http.StatusText(status))
	}()
	if r.Method == "OPTIONS" {
		w.Header().Set("Content-Type", "text/html; charset=ascii")
		status = http.StatusOK
		w.WriteHeader(status)
		return
	}

	switch r.URL.Path {
	case "/signin":
		h.handleSignin(w, r, &status)
		return
	case "/register":
		h.handleRegister(w, r, &status)
		return
	case "/refreshAuth":
		h.handleRefreshAuth(w, r, &status)
		return
	}

	user := h.bearerMiddleware(w, r, &status)
	if user == nil {
		status = http.StatusUnauthorized
		w.WriteHeader(status)
		return
	}
	switch r.URL.Path {
	case "/chessMove":
		h.handleChessMove(w, r, user, &status)
	default:
		status = http.StatusNotFound
		http.Error(w, "Not found", status)
	}
}

func (h *RHandler) handleRefreshAuth(w http.ResponseWriter, r *http.Request, status *int) {
	if r.Method != http.MethodPost {
		*status = http.StatusMethodNotAllowed
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	oldToken := r.Header.Get("Authorization")
	if oldToken == "" {
		*status = http.StatusUnauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	oldToken = strings.TrimPrefix(oldToken, "Bearer ")
	if oldToken == "" {
		*status = http.StatusUnauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := context.Background()
	query := h.Client.Collection("users").Where("token", "==", oldToken).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		*status = http.StatusInternalServerError
		log.Println("Could not query user:", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if len(docs) == 0 {
		*status = http.StatusUnauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	createdAt := docs[0].Data()["tokenCreatedAt"].(int64)
	if time.Now().Unix()-createdAt > tokenTTLSeconds {
		*status = http.StatusUnauthorized
		http.Error(w, "Expired", http.StatusUnauthorized)
		return
	}
	// Generate token and store it in the database.
	token := uuid.New().String()
	createdAt = time.Now().Unix()
	userData := docs[0].Data()
	userData["token"] = token
	userData["tokenCreatedAt"] = createdAt
	if _, err := docs[0].Ref.Set(ctx, userData); err != nil {
		log.Println("Could not update user token:", err)
		*status = http.StatusInternalServerError
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	var credits float64
	if v, ok := userData["credits"].(float64); ok {
		credits = v
	} else if v, ok := userData["credits"].(int); ok {
		credits = float64(v)
	}
	resp := UserResponse{
		Email:   userData["email"].(string),
		Credits: credits,
		Token:   token,
	}
	json.NewEncoder(w).Encode(resp)
	*status = http.StatusOK
}

func (h *RHandler) handleRegister(w http.ResponseWriter, r *http.Request, status *int) {
	if r.Method != http.MethodPost {
		*status = http.StatusMethodNotAllowed
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		*status = http.StatusBadRequest
		http.Error(w, "Could not parse the JSON request", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		*status = http.StatusBadRequest
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Check if email already exists
	query := h.Client.Collection("users").Where("email", "==", req.Email).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		log.Println("Could not query user:", err)
		*status = http.StatusInternalServerError
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if len(docs) > 0 {
		*status = http.StatusConflict
		http.Error(w, "Email already registered", http.StatusConflict)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Println("Could not hash password:", err)
		*status = http.StatusInternalServerError
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Create user document
	userData := map[string]any{
		"email":    req.Email,
		"password": string(hashedPassword),
		"credits":  10.0, // Starting credits
	}

	_, _, err = h.Client.Collection("users").Add(ctx, userData)
	if err != nil {
		log.Println("Could not create user:", err)
		*status = http.StatusInternalServerError
		http.Error(w, "Could not create user", http.StatusInternalServerError)
		return
	}
	var credits float64
	if v, ok := userData["credits"].(float64); ok {
		credits = v
	} else if v, ok := userData["credits"].(int); ok {
		credits = float64(v)
	}

	resp := UserResponse{
		Email:   req.Email,
		Credits: credits,
	}
	json.NewEncoder(w).Encode(resp)
	*status = http.StatusOK
}

func (h *RHandler) handleSignin(w http.ResponseWriter, r *http.Request, status *int) {
	if r.Method != http.MethodPost {
		*status = http.StatusMethodNotAllowed
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		*status = http.StatusBadRequest
		http.Error(w, "Could not parse the JSON request", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Find user
	query := h.Client.Collection("users").Where("email", "==", req.Email).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		log.Println("Could not query user:", err)
		*status = http.StatusInternalServerError
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if len(docs) == 0 {
		*status = http.StatusUnauthorized
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	// Verify password
	userData := docs[0].Data()
	storedPassword := userData["password"].(string)
	if err := bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(req.Password)); err != nil {
		*status = http.StatusUnauthorized
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	// Generate token and store it in the database.
	token := uuid.New().String()
	createdAt := time.Now().Unix()
	userData["token"] = token
	userData["tokenCreatedAt"] = createdAt
	_, err = docs[0].Ref.Set(ctx, userData)
	if err != nil {
		log.Println("Could not update user token:", err)
		*status = http.StatusInternalServerError
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	var credits float64
	if v, ok := userData["credits"].(float64); ok {
		credits = v
	} else if v, ok := userData["credits"].(int); ok {
		credits = float64(v)
	}

	// Here you might want to generate a JWT token
	resp := UserResponse{
		Email:   req.Email,
		Credits: credits,
		Token:   token,
	}
	json.NewEncoder(w).Encode(resp)
	*status = http.StatusOK
}

func (h *RHandler) decreaseCredits(n float64, user *UserResponse) error {
	ctx := context.Background()
	query := h.Client.Collection("users").Where("email", "==", user.Email).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		log.Println("Could not query user:", err)
		return fmt.Errorf("database error: %v", err)
	}
	if len(docs) == 0 {
		log.Println("User not found")
		return fmt.Errorf("user not found")
	}

	userData := docs[0].Data()
	credits := userData["credits"].(float64)
	if credits < n {
		log.Println("Insufficient credits")
		return fmt.Errorf("insufficient credits")
	}
	userData["credits"] = credits - n
	_, err = docs[0].Ref.Set(ctx, userData)
	if err != nil {
		log.Println("Could not update user credits:", err)
		return fmt.Errorf("server error: %v", err)
	}
	return nil
}

func (h *RHandler) handleChessMove(w http.ResponseWriter, r *http.Request, user *UserResponse, status *int) {
	if r.Method != http.MethodPost {
		*status = http.StatusMethodNotAllowed
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChessMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		*status = http.StatusBadRequest
		http.Error(w, "Could not parse the JSON request", http.StatusBadRequest)
		return
	}

	if err := h.decreaseCredits(1.0, user); err != nil {
		*status = http.StatusPaymentRequired
		http.Error(w, "Insufficient credits", http.StatusPaymentRequired)
		return
	}
	move, reason, err := models.CallLLM(req.Board, req.Provider, req.Model, req.AvailableMoves, req.InvalidMoves)
	if err != nil {
		*status = http.StatusInternalServerError
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ChessMoveResponse{
		Move:   move,
		Reason: reason,
	}
	json.NewEncoder(w).Encode(resp)
	*status = http.StatusOK
}

func (h *RHandler) bearerMiddleware(w http.ResponseWriter, r *http.Request, status *int) *UserResponse {
	txt := r.Header.Get("Authorization")
	if txt == "" {
		*status = http.StatusUnauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	token := strings.TrimPrefix(txt, "Bearer ")
	if token == "" {
		*status = http.StatusUnauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	ctx := context.Background()
	query := h.Client.Collection("users").Where("token", "==", token).Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		*status = http.StatusInternalServerError
		http.Error(w, "Database error", http.StatusInternalServerError)
		return nil
	}
	if len(docs) == 0 {
		*status = http.StatusUnauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}
	createdAt := docs[0].Data()["tokenCreatedAt"].(int64)
	if time.Now().Unix()-createdAt > tokenTTLSeconds {
		*status = http.StatusUnauthorized
		http.Error(w, "Expired", http.StatusUnauthorized)
		return nil
	}
	return &UserResponse{
		Email:   docs[0].Data()["email"].(string),
		Credits: docs[0].Data()["credits"].(float64),
		Token:   token,
	}
}
