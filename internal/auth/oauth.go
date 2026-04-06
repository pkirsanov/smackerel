package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	params := url.Values{
		"client_id":     {g.Config.ClientID},
		"redirect_uri":  {g.Config.RedirectURL},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
		"response_type": {"code"},
	}
	return fmt.Sprintf("%s?%s", g.Config.AuthEndpoint, params.Encode())
}

// ExchangeCode exchanges an authorization code for tokens.
func (g *GenericOAuth2) ExchangeCode(ctx context.Context, code string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {g.Config.RedirectURL},
		"client_id":     {g.Config.ClientID},
		"client_secret": {g.Config.ClientSecret},
	}
	return g.tokenRequest(ctx, data)
}

// RefreshToken refreshes an expired access token.
func (g *GenericOAuth2) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {g.Config.ClientID},
		"client_secret": {g.Config.ClientSecret},
	}
	return g.tokenRequest(ctx, data)
}

// tokenRequest makes a POST to the token endpoint and parses the response.
func (g *GenericOAuth2) tokenRequest(ctx context.Context, data url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Config.TokenEndpoint,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request to %s: %w", g.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d for %s", resp.StatusCode, g.Name)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	var scopes []string
	if tokenResp.Scope != "" {
		scopes = strings.Split(tokenResp.Scope, " ")
	}

	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    tokenResp.TokenType,
		Scopes:       scopes,
	}, nil
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
