package port

import (
	"context"
	"time"

	"github.com/araihu/manja/domain"
)

type Clock interface {
	Now(context.Context) time.Time
}

type IdentifierGenerator interface {
	NewID(context.Context, string) (string, error)
}

type Cache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

type ActorResolver interface {
	Actor(context.Context) (domain.Actor, error)
}

type Authorizer interface {
	Authorize(context.Context, domain.Actor, string, string) error
}
