package tools

import "context"

// Registrar is the narrow boundary later scopes use to register spec-037 agent
// tools without leaking provider calls into API, web, scheduler, or Telegram.
type Registrar interface {
	Register(ctx context.Context) error
}
