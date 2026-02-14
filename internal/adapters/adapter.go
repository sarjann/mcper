package adapters

import (
	"context"

	"github.com/sarjann/mcper/internal/model"
)

type Adapter interface {
	Name() string
	Path() string
	UpsertServers(context.Context, map[string]model.MCPServerSpec) error
	RemoveServers(context.Context, []string) error
	ListServers(context.Context) (map[string]model.MCPServerSpec, error)
}
