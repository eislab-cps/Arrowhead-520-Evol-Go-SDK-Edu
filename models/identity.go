package models

// Credentials is the credentials object of a login request.
type Credentials struct {
	Password string `json:"password"`
}

// LoginRequest is the body of C1, POST /authentication/identity/login.
type LoginRequest struct {
	SystemName  string      `json:"systemName"`
	Credentials Credentials `json:"credentials"`
}

// LoginResponse is the 201 body of C1.
type LoginResponse struct {
	Token          string `json:"token"`
	SystemName     string `json:"systemName"`
	ExpirationTime string `json:"expirationTime"`
	Sysop          bool   `json:"sysop"`
}
