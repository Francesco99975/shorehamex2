package views

import (
	"context"
	"io"

	"github.com/Francesco99975/shorehamex2/internal/config"
	"github.com/a-h/templ"
)

func WithLayout(layout func(config.Site) templ.Component, site config.Site, content templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return layout(site).Render(templ.WithChildren(ctx, content), w)
	})
}
