package auth

import (
	"context"
	"fmt"
	"time"
)

// Token represents an OAuth2 token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scopes       []string  `json:"scopes"`
}

// IsExpired returns true if the token has expired.
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// OAuth2Provider defines the OAuth2 abstraction for connectors.
type OAuth2Provider interface {
	// AuthURL returns the authorization URL for the consent screen.
	AuthURL(scopes []string, state string) string

	// ExchangeCode exchanges an authorization code for tokens.
	ExchangeCode(ctx context.Context, code string) (*Token, error)

	// RefreshToken refreshes an expired access token.
	RefreshToken(ctx context.Context, refreshToken string) (*Token, error)

	// ProviderName returns the name of this provider.
	ProviderName() string
}

// OAuth2Config holds the configuration for an OAuth2 provider.
type OAuth2Config struct {
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	AuthEndpoint  string
	TokenEndpoint string
}

// GenericOAuth2 implements OAuth2Provider for generic OAuth2 providers.
type GenericOAuth2 struct {
	Config OAuth2Config
	Name   string
}

// NewGenericOAuth2 creates a new generic OAuth2 provider.
func NewGenericOAuth2(name string, cfg OAuth2Config) *GenericOAuth2 {
	return &GenericOAuth2{Config: cfg, Name: name}
}

// ProviderName returns the provider name.
func (g *GenericOAuth2) ProviderName() string {
	return g.Name
}

// AuthURL builds the authorization URL.
func (g *GenericOAuth2) AuthURL(scopes []string, state string) string {
	scopeStr := ""
	for i, s := range scopes {
		if i > 0 {
			scopeStr += " "
		}
		scopeStr += s
	}

	return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&state=%s&response_type=code",
		g.Config.AuthEndpoint, g.Config.ClientID, g.Config.RedirectURL, scopeStr, state)
}

// ExchangeCode exchanges an authorization code for tokens.
func (g *GenericOAuth2) ExchangeCode(ctx context.Context, code string) (*Token, error) {
	// In a real implementation, this would make an HTTP call to the token endpoint
	return nil, fmt.Errorf("ExchangeCode not implemented for generic provider %s", g.Name)
}

// RefreshToken refreshes an expired access token.
func (g *GenericOAuth2) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	// In a real implementation, this would make an HTTP call
	return nil, fmt.Errorf("RefreshToken not implemented for generic provider %s", g.Name)
}

// GoogleOAuth2Scopes returns the scopes needed for Google services.
// Single consent screen covers Gmail IMAP + Calendar + YouTube.
func GoogleOAuth2Scopes() []string {
	return []string{
		"https://mail.google.com/",
		"https://www.googleapis.com/auth/calendar.readonly",
		"https://www.googleapis.com/auth/youtube.readonly",
	}
}
