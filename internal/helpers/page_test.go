// render_test.go
package helpers

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ——————— FAKE TEMPL COMPONENT (NO .templ FILES NEEDED) ———————
type fakeComponent struct {
	renderFunc func(ctx context.Context, w Writer) error
}

func (f fakeComponent) Render(ctx context.Context, w Writer) error {
	if f.renderFunc != nil {
		return f.renderFunc(ctx, w)
	}
	_, err := w.Write([]byte("<p>hello from fake</p>"))
	return err
}

// Writer is the minimal interface templ uses
type Writer = io.Writer

// ——————— SUCCESS CASE ———————
func TestRenderHTML_Success(t *testing.T) {
	t.Parallel()

	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			_, err := w.Write([]byte("<div>test content</div>"))
			return err
		},
	}

	html, err := RenderHTML(comp)
	require.NoError(t, err)
	assert.Equal(t, []byte("<div>test content</div>"), html)
}

// ——————— ERROR CASE ———————
func TestRenderHTML_ErrorPropagates(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("render failed")

	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			return expectedErr
		},
	}

	html, err := RenderHTML(comp)
	assert.ErrorIs(t, err, expectedErr)
	assert.Empty(t, html)
}

// ——————— MUSTRENDER PANICS ON ERROR ———————
func TestMustRenderHTML_PanicsOnError(t *testing.T) {
	t.Parallel()

	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			return errors.New("boom")
		},
	}

	assert.PanicsWithError(t, "boom", func() {
		MustRenderHTML(comp)
	})
}

// ——————— MUSTRENDER RETURNS BYTES ON SUCCESS ———————
func TestMustRenderHTML_Success(t *testing.T) {
	t.Parallel()

	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			_, err := w.Write([]byte("<h1>success</h1>"))
			return err
		},
	}

	html := MustRenderHTML(comp)
	assert.Equal(t, []byte("<h1>success</h1>"), html)
}

// ——————— CONCURRENT SAFETY (1000 GOROUTINES) ———————
func TestRenderHTML_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			_, err := w.Write([]byte("safe"))
			return err
		},
	}

	var wg sync.WaitGroup
	for range 1000 {
		wg.Go(func() {
			html, err := RenderHTML(comp)
			assert.NoError(t, err)
			assert.Equal(t, []byte("safe"), html)
		})
	}
	wg.Wait()
}

// ——————— BENCHMARKS ———————
func BenchmarkRenderHTML(b *testing.B) {
	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			_, err := w.Write([]byte("<p>bench</p>"))
			return err
		},
	}

	for b.Loop() {
		_, _ = RenderHTML(comp)
	}
}

func BenchmarkMustRenderHTML(b *testing.B) {
	comp := fakeComponent{
		renderFunc: func(ctx context.Context, w Writer) error {
			_, err := w.Write([]byte("<p>bench</p>"))
			return err
		},
	}

	for b.Loop() {
		_ = MustRenderHTML(comp)
	}
}
