package helpers

import (
	"bytes"
	"context"

	"github.com/a-h/templ"
)

func RenderHTML(page templ.Component) ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	err := page.Render(context.Background(), buf)

	if err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func MustRenderHTML(page templ.Component) []byte {
	buf := bytes.NewBuffer(nil)

	err := page.Render(context.Background(), buf)

	if err != nil {
		panic(err)
	}

	return buf.Bytes()
}
